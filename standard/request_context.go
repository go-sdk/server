package standard

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-sdk/core/logx"
	"github.com/go-sdk/core/seq"
	grpclogging "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

const (
	requestIDHeader        = "X-Request-ID"
	requestIDMetadataKey   = "x-request-id"
	clientIPMetadataKey    = "x-client-ip"
	contentTypeMetadataKey = "x-content-type"
	userAgentMetadataKey   = "x-user-agent"
	depthMetadataKey       = "x-depth"
	depthHeader            = "X-Depth"
	requestIDMaxLength     = 128
)

func normalizeRequestID(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || len(requestID) > requestIDMaxLength {
		return seq.UUID()
	}
	return requestID
}

// RequestID 返回标准中间件写入 context 的请求标识。
func RequestID(ctx context.Context) string {
	return FromContext(ctx).RequestID()
}

func httpRequestContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := normalizeRequestID(r.Header.Get(requestIDHeader))
		w.Header().Set(requestIDHeader, requestID)
		ctx := requestContext(
			r.Context(),
			requestID,
			remoteIP(r.RemoteAddr),
			r.Header.Get("Content-Type"),
			r.UserAgent(),
			parseDepth(r.Header.Get(depthHeader)),
		)
		ctx = contextWithAuthorization(ctx, r.Header.Get("Authorization"))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func unaryRequestContextInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, requestID := grpcRequestContext(ctx)
		_ = grpc.SetHeader(ctx, metadata.Pairs(requestIDMetadataKey, requestID))
		return handler(ctx, req)
	}
}

func streamRequestContextInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, requestID := grpcRequestContext(stream.Context())
		_ = stream.SetHeader(metadata.Pairs(requestIDMetadataKey, requestID))
		return handler(srv, &contextServerStream{ServerStream: stream, ctx: ctx})
	}
}

func grpcRequestContext(ctx context.Context) (context.Context, string) {
	requestID := normalizeRequestID(firstMetadataValue(ctx, requestIDMetadataKey))
	clientIP := firstMetadataValue(ctx, clientIPMetadataKey)
	if clientIP == "" {
		if client, ok := peer.FromContext(ctx); ok {
			clientIP = remoteIP(client.Addr.String())
		}
	}
	contentType := firstMetadataValue(ctx, contentTypeMetadataKey)
	if contentType == "" {
		contentType = firstMetadataValue(ctx, "content-type")
	}
	if contentType == "" {
		contentType = "application/grpc"
	}
	userAgent := firstMetadataValue(ctx, userAgentMetadataKey)
	if userAgent == "" {
		userAgent = firstMetadataValue(ctx, "user-agent")
	}
	ctx = requestContext(ctx, requestID, clientIP, contentType, userAgent, parseDepth(firstMetadataValue(ctx, depthMetadataKey)))
	ctx = contextWithAuthorization(ctx, firstMetadataValue(ctx, "authorization"))
	return ctx, requestID
}

func requestContext(ctx context.Context, requestID, clientIP, contentType, userAgent string, depth int) context.Context {
	ctx = NewContext(ctx,
		RequestIDKey, requestID,
		ClientIPKey, clientIP,
		ContentTypeKey, contentType,
		UserAgentKey, userAgent,
		DepthKey, depth,
	)
	logger := logx.Ctx(ctx).With().Str(RequestIDKey, requestID).Int("depth", depth).Logger()
	return logger.WithContext(ctx)
}

func firstMetadataValue(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func requestLogFields(ctx context.Context) grpclogging.Fields {
	requestContext := FromContext(ctx)
	return grpclogging.Fields{
		RequestIDKey, requestContext.RequestID(),
		"client_ip", requestContext.ClientIP(),
		"content_type", requestContext.ContentType(),
		"user_agent", requestContext.UserAgent(),
		"depth", requestContext.Depth(),
	}
}

func parseDepth(value string) int {
	depth, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || depth < 0 {
		return 0
	}
	return depth
}

func remoteIP(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}
	return address
}
