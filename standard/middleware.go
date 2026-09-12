package standard

import (
	"context"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"buf.build/go/protovalidate"
	"github.com/go-sdk/core/logx"
	"github.com/go-sdk/core/seq"
	grpcmiddleware "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	grpclogging "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	serveroptions "github.com/go-sdk/server/options"
)

const (
	requestIDHeader      = "X-Request-ID"
	requestIDMetadataKey = "x-request-id"
	requestIDMaxLength   = 128
)

type requestIDContextKey struct{}

type contextServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *contextServerStream) Context() context.Context { return s.ctx }

type statusResponseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (w *statusResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.size += n
	return n, err
}

func newValidator() (protovalidate.Validator, error) {
	return protovalidate.New()
}

func requestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

// RequestID 返回标准 middleware 写入 context 的请求标识。
func RequestID(ctx context.Context) string {
	return requestIDFromContext(ctx)
}

func contextWithRequestID(ctx context.Context, requestID string) context.Context {
	ctx = context.WithValue(ctx, requestIDContextKey{}, requestID)
	logger := logx.Ctx(ctx).With().Str("request_id", requestID).Logger()
	return logger.WithContext(ctx)
}

func normalizeRequestID(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || len(requestID) > requestIDMaxLength {
		return seq.UUID()
	}
	return requestID
}

func httpRequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := normalizeRequestID(r.Header.Get(requestIDHeader))
		w.Header().Set(requestIDHeader, requestID)
		next.ServeHTTP(w, r.WithContext(contextWithRequestID(r.Context(), requestID)))
	})
}

func httpAccessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		writer := &statusResponseWriter{ResponseWriter: w}
		next.ServeHTTP(writer, r)
		statusCode := writer.status
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		logx.Ctx(r.Context()).Info().
			Str("request_id", requestIDFromContext(r.Context())).
			Str("http_method", r.Method).
			Str("http_path", r.URL.Path).
			Int("http_status", statusCode).
			Int("response_size", writer.size).
			Str("remote_address", r.RemoteAddr).
			Dur("duration", time.Since(startedAt)).
			Msg("http request completed")
	})
}

func httpRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logx.Ctx(r.Context()).Error().
					Str("request_id", requestIDFromContext(r.Context())).
					Interface("panic", recovered).
					Bytes("stack", debug.Stack()).
					Msg("http handler panic recovered")
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func gatewayRequestMetadata(ctx context.Context, _ *http.Request) metadata.MD {
	requestID := requestIDFromContext(ctx)
	if requestID == "" {
		return nil
	}
	return metadata.Pairs(requestIDMetadataKey, requestID)
}

func unaryRequestIDInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, requestID := grpcRequestContext(ctx)
		_ = grpc.SetHeader(ctx, metadata.Pairs(requestIDMetadataKey, requestID))
		return handler(ctx, req)
	}
}

func streamRequestIDInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, requestID := grpcRequestContext(stream.Context())
		_ = stream.SetHeader(metadata.Pairs(requestIDMetadataKey, requestID))
		return handler(srv, &contextServerStream{ServerStream: stream, ctx: ctx})
	}
}

func grpcRequestContext(ctx context.Context) (context.Context, string) {
	var requestID string
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		values := md.Get(requestIDMetadataKey)
		if len(values) > 0 {
			requestID = values[0]
		}
	}
	requestID = normalizeRequestID(requestID)
	return contextWithRequestID(ctx, requestID), requestID
}

func requestIDLogFields(ctx context.Context) grpclogging.Fields {
	requestID := requestIDFromContext(ctx)
	if requestID == "" {
		return nil
	}
	return grpclogging.Fields{"request_id", requestID}
}

func shouldLogMethod(_ context.Context, callMeta grpcmiddleware.CallMeta) bool {
	descriptor, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(callMeta.Service))
	if err != nil {
		return true
	}
	service, ok := descriptor.(protoreflect.ServiceDescriptor)
	if !ok {
		return true
	}
	method := service.Methods().ByName(protoreflect.Name(callMeta.Method))
	if method == nil {
		return true
	}
	methodOptions, ok := method.Options().(*descriptorpb.MethodOptions)
	if !ok || !proto.HasExtension(methodOptions, serveroptions.E_Method) {
		return true
	}
	option, ok := proto.GetExtension(methodOptions, serveroptions.E_Method).(*serveroptions.MethodOptions)
	return !ok || !option.GetSkipLog()
}

func recoveryHandler(ctx context.Context, recovered any) error {
	logx.Ctx(ctx).Error().
		Str("request_id", requestIDFromContext(ctx)).
		Interface("panic", recovered).
		Bytes("stack", debug.Stack()).
		Msg("grpc handler panic recovered")
	return status.Error(codes.Internal, "internal server error")
}
