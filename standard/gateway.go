package standard

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-sdk/core/errx"
	"google.golang.org/grpc/metadata"
)

// gatewayOutgoingHeaderMatcher 丢弃全部 gRPC 响应头：外层 HTTP 中间件已经写入 X-Request-Id，
// 若再透传 gRPC 响应头会因 grpc-gateway 使用 Header.Add 而产生重复响应头；SpanID 仅用于日志，不出现在响应中。
func gatewayOutgoingHeaderMatcher(_ string) (string, bool) {
	return "", false
}

func gatewayRequestMetadata(ctx context.Context, request *http.Request) metadata.MD {
	requestContext := FromContext(ctx)
	pairs := []string{
		traceIDMetadataKey, requestContext.TraceID(),
		clientIPMetadataKey, requestContext.ClientIP(),
		contentTypeMetadataKey, request.Header.Get("Content-Type"),
		userAgentMetadataKey, request.UserAgent(),
		depthMetadataKey, strconv.Itoa(requestContext.Depth() + 1),
	}
	if requestContext.authorization != "" {
		pairs = append(pairs, "authorization", requestContext.authorization)
	}
	if requestContext.AcceptLanguage() != "" {
		pairs = append(pairs, acceptLanguageMetadataKey, requestContext.AcceptLanguage())
	}
	return metadata.Pairs(pairs...)
}

func (s *Server) registerGateway(ctx context.Context, listener net.Listener) error {
	if len(s.config.gatewayRegisters) == 0 {
		return nil
	}
	endpoint, err := s.gatewayEndpoint(listener)
	if err != nil {
		return err
	}
	dialOptions, err := s.gatewayDialOptions(endpoint)
	if err != nil {
		return err
	}
	for _, register := range s.config.gatewayRegisters {
		if err := register(ctx, s.gatewayMux, endpoint, dialOptions); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) gatewayEndpoint(listener net.Listener) (string, error) {
	if s.config.gatewayEndpoint != "" {
		return s.config.gatewayEndpoint, nil
	}
	network := listener.Addr().Network()
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return "", errx.New("a gateway endpoint is required for a non-tcp listener")
	}
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return "", errx.Wrap(err, "resolve gateway endpoint")
	}
	if host == "" || isUnspecifiedHost(host) {
		host = loopbackHost(network, host)
	}
	return net.JoinHostPort(host, port), nil
}

func isUnspecifiedHost(host string) bool {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsUnspecified()
}

func loopbackHost(network, host string) string {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if network == "tcp6" || ip != nil && ip.To4() == nil {
		return "::1"
	}
	return "127.0.0.1"
}
