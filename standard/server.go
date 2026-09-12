package standard

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/elastic/gmux"
	"github.com/go-sdk/core/errx"
	"github.com/go-sdk/core/lifex"
	"github.com/go-sdk/core/logx"
	grpclogging "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpcprotovalidate "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/protovalidate"
	grpcrecovery "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	grpcselector "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/selector"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	grpchealthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// Server 在同一个端口提供原生 gRPC 和 grpc-gateway HTTP 服务。
type Server struct {
	config       config
	grpcServer   *grpc.Server
	gatewayMux   *runtime.ServeMux
	httpServer   *http.Server
	grpcListener net.Listener
	healthServer *health.Server

	mu            sync.Mutex
	state         serverState
	gatewayCancel func()
	stopDone      chan struct{}
	stopErr       error
}

// New 使用 Options 初始化服务，并将启动和停止函数注册到 lifex。
func New(opts ...Option) (*Server, error) {
	cfg := defaultConfig()
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

	grpcServer, err := newGRPCServer(cfg)
	if err != nil {
		return nil, err
	}
	for _, register := range cfg.grpcRegisters {
		register(grpcServer)
	}
	healthServer := health.NewServer()
	grpchealthv1.RegisterHealthServer(grpcServer, healthServer)
	if cfg.reflection {
		reflection.Register(grpcServer)
	}
	for service := range grpcServer.GetServiceInfo() {
		healthServer.SetServingStatus(service, grpchealthv1.HealthCheckResponse_SERVING)
	}

	gatewayOptions := []runtime.ServeMuxOption{runtime.WithMetadata(gatewayRequestMetadata)}
	gatewayOptions = append(gatewayOptions, cfg.gatewayOptions...)
	gatewayOptions = append(gatewayOptions, runtime.WithForwardResponseRewriter(gatewayResponseRewriter))
	gatewayMux := runtime.NewServeMux(gatewayOptions...)
	handler := httpRequestContextMiddleware(httpAccessLogMiddleware(httpRecoveryMiddleware(gatewayMux)))
	httpServer := newHTTPServer(cfg, handler)
	grpcListener, err := gmux.ConfigureServer(httpServer, nil)
	if err != nil {
		if closeErr := httpServer.Close(); closeErr != nil {
			logx.Error().Err(closeErr).Msg("close http server after gmux configuration failure")
		}
		return nil, errx.Wrap(err, "configure gmux")
	}

	server := &Server{
		config:       cfg,
		grpcServer:   grpcServer,
		gatewayMux:   gatewayMux,
		httpServer:   httpServer,
		grpcListener: grpcListener,
		healthServer: healthServer,
		state:        serverStateNew,
		stopDone:     make(chan struct{}),
	}
	lifex.OnInit(server.Start)
	lifex.OnDeinit(server.Stop)
	return server, nil
}

// HandlePath 注册无法通过 google.api.http 表达的额外 HTTP 接口。
func (s *Server) HandlePath(method, path string, handler runtime.HandlerFunc) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != serverStateNew {
		return errx.New("http routes must be registered before the server starts")
	}
	if err := s.gatewayMux.HandlePath(method, path, jwtHTTPHandler(s.config.jwtSecret, handler)); err != nil {
		return errx.Wrap(err, "register http route")
	}
	return nil
}

func newGRPCServer(cfg config) (*grpc.Server, error) {
	validator, err := newValidator()
	if err != nil {
		return nil, errx.Wrap(err, "initialize protovalidate")
	}
	logger := cfg.logger
	if logger == nil {
		logger = newLogger()
	}
	recoveryOption := grpcrecovery.WithRecoveryHandlerContext(recoveryHandler)
	loggingMatcher := grpcselector.MatchFunc(shouldLogMethod)
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		unaryRequestContextInterceptor(),
		grpcselector.UnaryServerInterceptor(
			grpclogging.UnaryServerInterceptor(logger, grpclogging.WithFieldsFromContext(requestLogFields)),
			loggingMatcher,
		),
		unaryPayloadLoggingInterceptor(),
		unaryJWTAuthInterceptor(cfg.jwtSecret),
		grpcprotovalidate.UnaryServerInterceptor(validator),
	}
	unaryInterceptors = append(unaryInterceptors, cfg.unaryInterceptors...)
	unaryInterceptors = append(unaryInterceptors, unaryErrorConverterInterceptor(cfg.errorConverters))
	unaryInterceptors = append(unaryInterceptors, grpcrecovery.UnaryServerInterceptor(recoveryOption))
	streamInterceptors := []grpc.StreamServerInterceptor{
		streamRequestContextInterceptor(),
		grpcselector.StreamServerInterceptor(
			grpclogging.StreamServerInterceptor(logger, grpclogging.WithFieldsFromContext(requestLogFields)),
			loggingMatcher,
		),
		streamPayloadLoggingInterceptor(),
		streamJWTAuthInterceptor(cfg.jwtSecret),
		grpcprotovalidate.StreamServerInterceptor(validator),
	}
	streamInterceptors = append(streamInterceptors, cfg.streamInterceptors...)
	streamInterceptors = append(streamInterceptors, streamErrorConverterInterceptor(cfg.errorConverters))
	streamInterceptors = append(streamInterceptors, grpcrecovery.StreamServerInterceptor(recoveryOption))

	grpcOptions := append([]grpc.ServerOption{}, cfg.grpcOptions...)
	grpcOptions = append(grpcOptions,
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	)
	return grpc.NewServer(grpcOptions...), nil
}

func newHTTPServer(cfg config, handler http.Handler) *http.Server {
	server := &http.Server{
		Addr:              cfg.address,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}
	if cfg.tlsConfig != nil {
		server.TLSConfig = cfg.tlsConfig.Clone()
	}
	if cfg.certificate != nil {
		if server.TLSConfig == nil {
			server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		}
		server.TLSConfig.Certificates = []tls.Certificate{*cfg.certificate}
		server.TLSConfig.GetCertificate = nil
	} else if cfg.certFile != "" && server.TLSConfig != nil {
		server.TLSConfig.Certificates = nil
		server.TLSConfig.GetCertificate = nil
	}
	for _, opt := range cfg.httpServerOptions {
		opt(server)
	}
	// Server 的 Handler 始终由内部 Gateway 和标准 middleware 管理。
	server.Handler = handler
	return server
}
