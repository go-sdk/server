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

func gatewayRequestMetadata(ctx context.Context, request *http.Request) metadata.MD {
	requestContext := FromContext(ctx)
	pairs := []string{
		requestIDMetadataKey, requestContext.RequestID(),
		clientIPMetadataKey, requestContext.ClientIP(),
		contentTypeMetadataKey, request.Header.Get("Content-Type"),
		userAgentMetadataKey, request.UserAgent(),
		depthMetadataKey, strconv.Itoa(requestContext.Depth() + 1),
	}
	if requestContext.authorization != "" {
		pairs = append(pairs, "authorization", requestContext.authorization)
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
