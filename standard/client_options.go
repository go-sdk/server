package standard

import (
	"crypto/tls"

	"github.com/go-sdk/core/errx"
	grpclogging "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"google.golang.org/grpc"
)

// ClientOption 配置标准 gRPC ClientConn。
type ClientOption func(*clientConfig) error

type clientConfig struct {
	dialOptions        []grpc.DialOption
	unaryInterceptors  []grpc.UnaryClientInterceptor
	streamInterceptors []grpc.StreamClientInterceptor
	logger             grpclogging.Logger
	tlsConfig          *tls.Config
	rootCertFile       string
	rootCertPEM        []byte
	serverName         string
}

func (c clientConfig) validate() error {
	if c.tlsConfig != nil && (c.rootCertFile != "" || len(c.rootCertPEM) != 0) {
		return errx.New("client tls config and root certificate must not be specified together")
	}
	if c.rootCertFile != "" && len(c.rootCertPEM) != 0 {
		return errx.New("client root certificate file and pem must not be specified together")
	}
	return nil
}

func WithClientDialOptions(opts ...grpc.DialOption) ClientOption {
	return func(c *clientConfig) error {
		c.dialOptions = append(c.dialOptions, opts...)
		return nil
	}
}

func WithClientUnaryInterceptors(interceptors ...grpc.UnaryClientInterceptor) ClientOption {
	return func(c *clientConfig) error {
		c.unaryInterceptors = append(c.unaryInterceptors, interceptors...)
		return nil
	}
}

func WithClientStreamInterceptors(interceptors ...grpc.StreamClientInterceptor) ClientOption {
	return func(c *clientConfig) error {
		c.streamInterceptors = append(c.streamInterceptors, interceptors...)
		return nil
	}
}

func WithClientLogger(logger grpclogging.Logger) ClientOption {
	return func(c *clientConfig) error {
		if logger == nil {
			return errx.New("client logger must not be nil")
		}
		c.logger = logger
		return nil
	}
}

// WithClientTLSConfig 使用自定义 TLS 配置连接 gRPC Server。
func WithClientTLSConfig(tlsConfig *tls.Config) ClientOption {
	return func(c *clientConfig) error {
		if tlsConfig == nil {
			return errx.New("client tls config must not be nil")
		}
		c.tlsConfig = tlsConfig.Clone()
		return nil
	}
}

func WithClientRootCertificate(certFile string) ClientOption {
	return func(c *clientConfig) error {
		if certFile == "" {
			return errx.New("client root certificate file must not be empty")
		}
		c.rootCertFile = certFile
		return nil
	}
}

func WithClientRootCertificatePEM(certPEM []byte) ClientOption {
	return func(c *clientConfig) error {
		if len(certPEM) == 0 {
			return errx.New("client root certificate pem must not be empty")
		}
		c.rootCertPEM = append([]byte(nil), certPEM...)
		return nil
	}
}

func WithClientServerName(serverName string) ClientOption {
	return func(c *clientConfig) error {
		c.serverName = serverName
		return nil
	}
}
