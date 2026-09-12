package standard

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"strings"

	"github.com/go-sdk/core/errx"
	"github.com/go-sdk/core/lifex"
	grpclogging "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpcselector "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/selector"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// NewClient 创建由 lifex 管理的标准 gRPC ClientConn，默认使用明文连接。
func NewClient(target string, opts ...ClientOption) (*grpc.ClientConn, error) {
	if strings.TrimSpace(target) == "" {
		return nil, errx.New("grpc client target must not be empty")
	}
	cfg := clientConfig{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	transportCredentials, err := clientTransportCredentials(cfg)
	if err != nil {
		return nil, err
	}
	logger := cfg.logger
	if logger == nil {
		logger = newLogger()
	}
	unaryInterceptors := []grpc.UnaryClientInterceptor{
		unaryClientContextInterceptor(),
		grpcselector.UnaryClientInterceptor(
			grpclogging.UnaryClientInterceptor(logger, grpclogging.WithFieldsFromContext(requestLogFields)),
			grpcselector.MatchFunc(shouldLogMethod),
		),
		unaryClientPayloadLoggingInterceptor(),
	}
	unaryInterceptors = append(unaryInterceptors, cfg.unaryInterceptors...)
	streamInterceptors := []grpc.StreamClientInterceptor{
		streamClientContextInterceptor(),
		grpcselector.StreamClientInterceptor(
			grpclogging.StreamClientInterceptor(logger, grpclogging.WithFieldsFromContext(requestLogFields)),
			grpcselector.MatchFunc(shouldLogMethod),
		),
		streamClientPayloadLoggingInterceptor(),
	}
	streamInterceptors = append(streamInterceptors, cfg.streamInterceptors...)
	dialOptions := append([]grpc.DialOption{}, cfg.dialOptions...)
	dialOptions = append(dialOptions,
		grpc.WithTransportCredentials(transportCredentials),
		grpc.WithChainUnaryInterceptor(unaryInterceptors...),
		grpc.WithChainStreamInterceptor(streamInterceptors...),
	)
	conn, err := grpc.NewClient(target, dialOptions...)
	if err != nil {
		return nil, errx.Wrap(err, "create grpc client")
	}
	lifex.OnDeinit(func() error { return closeClient(conn) })
	return conn, nil
}

func closeClient(conn *grpc.ClientConn) error {
	err := conn.Close()
	if status.Code(err) == codes.Canceled {
		return nil
	}
	return err
}

func clientTransportCredentials(cfg clientConfig) (credentials.TransportCredentials, error) {
	if cfg.tlsConfig != nil {
		tlsConfig := cfg.tlsConfig.Clone()
		if cfg.serverName != "" {
			tlsConfig.ServerName = cfg.serverName
		}
		return credentials.NewTLS(tlsConfig), nil
	}
	if cfg.rootCertFile == "" && len(cfg.rootCertPEM) == 0 {
		return insecure.NewCredentials(), nil
	}
	certPEM := cfg.rootCertPEM
	if cfg.rootCertFile != "" {
		var err error
		certPEM, err = os.ReadFile(cfg.rootCertFile)
		if err != nil {
			return nil, errx.Wrap(err, "read client root certificate")
		}
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(certPEM) {
		return nil, errx.New("parse client root certificate")
	}
	return credentials.NewTLS(&tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: cfg.serverName,
	}), nil
}
