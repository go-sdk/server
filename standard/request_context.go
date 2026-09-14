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
	// traceIDHeader 和 traceIDMetadataKey 是跨服务透传的线上协议：对外统一接收和返回 X-Request-Id。
	traceIDHeader             = "X-Request-ID"
	traceIDMetadataKey        = "x-request-id"
	clientIPMetadataKey       = "x-client-ip"
	contentTypeMetadataKey    = "x-content-type"
	userAgentMetadataKey      = "x-user-agent"
	acceptLanguageMetadataKey = "accept-language"
	depthMetadataKey          = "x-depth"
	depthHeader               = "X-Depth"
	traceIDMaxLength          = 128
)

func normalizeTraceID(traceID string) string {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" || len(traceID) > traceIDMaxLength {
		return seq.UUID()
	}
	return traceID
}

// TraceID 返回标准中间件写入 context 的链路追踪标识，跨服务透传。
func TraceID(ctx context.Context) string {
	return FromContext(ctx).TraceID()
}

// SpanID 返回标准中间件写入 context 的当前服务内单次请求处理标识。
func SpanID(ctx context.Context) string {
	return FromContext(ctx).SpanID()
}

func httpRequestContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := normalizeTraceID(r.Header.Get(traceIDHeader))
		spanID := newSpanID()
		w.Header().Set(traceIDHeader, traceID)
		ctx := requestContext(
			r.Context(),
			traceID,
			spanID,
			remoteIP(r.RemoteAddr),
			r.Header.Get("Content-Type"),
			r.UserAgent(),
			r.Header.Get("Accept-Language"),
			parseDepth(r.Header.Get(depthHeader)),
		)
		ctx = contextWithAuthorization(ctx, r.Header.Get("Authorization"))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func unaryRequestContextInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, traceID, _ := grpcRequestContext(ctx)
		_ = grpc.SetHeader(ctx, metadata.Pairs(traceIDMetadataKey, traceID))
		return handler(ctx, req)
	}
}

func streamRequestContextInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, traceID, _ := grpcRequestContext(stream.Context())
		_ = stream.SetHeader(metadata.Pairs(traceIDMetadataKey, traceID))
		return handler(srv, &contextServerStream{ServerStream: stream, ctx: ctx})
	}
}

func grpcRequestContext(ctx context.Context) (context.Context, string, string) {
	traceID := normalizeTraceID(firstMetadataValue(ctx, traceIDMetadataKey))
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
	acceptLanguage := firstMetadataValue(ctx, acceptLanguageMetadataKey)
	spanID := newSpanID()
	ctx = requestContext(ctx, traceID, spanID, clientIP, contentType, userAgent, acceptLanguage, parseDepth(firstMetadataValue(ctx, depthMetadataKey)))
	ctx = contextWithAuthorization(ctx, firstMetadataValue(ctx, "authorization"))
	return ctx, traceID, spanID
}

func requestContext(ctx context.Context, traceID, spanID, clientIP, contentType, userAgent, acceptLanguage string, depth int) context.Context {
	ctx = NewContext(ctx,
		TraceIDKey, traceID,
		SpanIDKey, spanID,
		ClientIPKey, clientIP,
		ContentTypeKey, contentType,
		UserAgentKey, userAgent,
		AcceptLanguageKey, acceptLanguage,
		DepthKey, depth,
	)
	logger := logx.Ctx(ctx).With().Str(TraceIDKey, traceID).Str(SpanIDKey, spanID).Int("depth", depth).Logger()
	return logger.WithContext(ctx)
}

// newSpanID 为每次进入服务的请求生成独立的处理标识，不透传给下游服务。
func newSpanID() string {
	return seq.UUIDShort()
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
		TraceIDKey, requestContext.TraceID(),
		SpanIDKey, requestContext.SpanID(),
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
