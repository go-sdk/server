package standard

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-sdk/core/errx"
	grpclogging "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
)

// GRPCRegisterFunc 向统一的 gRPC Server 注册业务服务。
type GRPCRegisterFunc func(grpc.ServiceRegistrar)

// GatewayRegisterFunc 注册由 google.api.http 生成的 Gateway 路由。
type GatewayRegisterFunc func(context.Context, *runtime.ServeMux, string, []grpc.DialOption) error

// HTTPServerOption 调整底层 HTTP Server 的标准参数，不允许替换 Handler。
type HTTPServerOption func(*http.Server)

// Option 配置 Server 的初始化参数。
type Option func(*config) error

type config struct {
	address            string
	listener           net.Listener
	gracefulTimeout    time.Duration
	certFile           string
	keyFile            string
	tlsConfig          *tls.Config
	gatewayEndpoint    string
	gatewayServerName  string
	gatewayDialOptions []grpc.DialOption
	gatewayOptions     []runtime.ServeMuxOption
	grpcOptions        []grpc.ServerOption
	httpServerOptions  []HTTPServerOption
	unaryInterceptors  []grpc.UnaryServerInterceptor
	streamInterceptors []grpc.StreamServerInterceptor
	grpcRegisters      []GRPCRegisterFunc
	gatewayRegisters   []GatewayRegisterFunc
	logger             grpclogging.Logger
}

func defaultConfig() config {
	return config{address: ":8080", gracefulTimeout: 5 * time.Second}
}

func (c config) validate() error {
	if c.listener == nil && strings.TrimSpace(c.address) == "" {
		return errx.New("address must not be empty when listener is not set")
	}
	if c.gracefulTimeout <= 0 {
		return errx.New("graceful timeout must be greater than zero")
	}
	if (c.certFile == "") != (c.keyFile == "") {
		return errx.New("certificate and private key files must be specified together")
	}
	for _, register := range c.grpcRegisters {
		if register == nil {
			return errx.New("grpc register function must not be nil")
		}
	}
	for _, register := range c.gatewayRegisters {
		if register == nil {
			return errx.New("gateway register function must not be nil")
		}
	}
	return nil
}

func WithAddress(address string) Option {
	return func(c *config) error { c.address = address; return nil }
}

func WithListener(listener net.Listener) Option {
	return func(c *config) error {
		if listener == nil {
			return errx.New("listener must not be nil")
		}
		c.listener = listener
		return nil
	}
}

func WithGracefulTimeout(timeout time.Duration) Option {
	return func(c *config) error { c.gracefulTimeout = timeout; return nil }
}

func WithCertificate(certFile, keyFile string) Option {
	return func(c *config) error { c.certFile, c.keyFile = certFile, keyFile; return nil }
}

func WithTLSConfig(tlsConfig *tls.Config) Option {
	return func(c *config) error {
		if tlsConfig == nil {
			return errx.New("tls config must not be nil")
		}
		c.tlsConfig = tlsConfig.Clone()
		return nil
	}
}

func WithGatewayEndpoint(endpoint string) Option {
	return func(c *config) error { c.gatewayEndpoint = endpoint; return nil }
}

func WithGatewayServerName(serverName string) Option {
	return func(c *config) error { c.gatewayServerName = serverName; return nil }
}

func WithGatewayDialOptions(opts ...grpc.DialOption) Option {
	return func(c *config) error {
		c.gatewayDialOptions = append(c.gatewayDialOptions, opts...)
		return nil
	}
}

func WithGatewayOptions(opts ...runtime.ServeMuxOption) Option {
	return func(c *config) error { c.gatewayOptions = append(c.gatewayOptions, opts...); return nil }
}

func WithGRPCServerOptions(opts ...grpc.ServerOption) Option {
	return func(c *config) error { c.grpcOptions = append(c.grpcOptions, opts...); return nil }
}

func WithHTTPServerOptions(opts ...HTTPServerOption) Option {
	return func(c *config) error { c.httpServerOptions = append(c.httpServerOptions, opts...); return nil }
}

func WithUnaryInterceptors(interceptors ...grpc.UnaryServerInterceptor) Option {
	return func(c *config) error {
		c.unaryInterceptors = append(c.unaryInterceptors, interceptors...)
		return nil
	}
}

func WithStreamInterceptors(interceptors ...grpc.StreamServerInterceptor) Option {
	return func(c *config) error {
		c.streamInterceptors = append(c.streamInterceptors, interceptors...)
		return nil
	}
}

func WithGRPCRegister(registers ...GRPCRegisterFunc) Option {
	return func(c *config) error { c.grpcRegisters = append(c.grpcRegisters, registers...); return nil }
}

func WithGatewayRegister(registers ...GatewayRegisterFunc) Option {
	return func(c *config) error {
		c.gatewayRegisters = append(c.gatewayRegisters, registers...)
		return nil
	}
}

func WithLogger(logger grpclogging.Logger) Option {
	return func(c *config) error {
		if logger == nil {
			return errx.New("logger must not be nil")
		}
		c.logger = logger
		return nil
	}
}
