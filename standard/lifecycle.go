package standard

import (
	"context"
	"net"
	"net/http"

	"github.com/go-sdk/core/errx"
	"github.com/go-sdk/core/lifex"
	"github.com/go-sdk/core/logx"
	"google.golang.org/grpc"
)

type serverState uint8

const (
	serverStateNew serverState = iota
	serverStateStarting
	serverStateRunning
	serverStateStopping
	serverStateStopped
)

// Start 完成监听和 Gateway 注册，然后异步启动同端口的 HTTP 与 gRPC 服务。
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != serverStateNew {
		return errx.New("server can only be started once")
	}
	s.state = serverStateStarting

	listener, err := s.listen()
	if err != nil {
		s.finishStart()
		return err
	}
	gatewayCtx, gatewayCancel := context.WithCancel(context.Background())
	if err := s.registerGateway(gatewayCtx, listener); err != nil {
		gatewayCancel()
		_ = listener.Close()
		s.finishStart()
		return errx.Wrap(err, "register gateway")
	}
	s.gatewayCancel = gatewayCancel
	s.state = serverStateRunning

	go s.serve("grpc", func() error {
		return s.grpcServer.Serve(s.grpcListener)
	})
	go s.serve("http", func() error {
		if s.usesTLS() {
			return s.httpServer.ServeTLS(listener, s.config.certFile, s.config.keyFile)
		}
		return s.httpServer.Serve(listener)
	})
	logx.Info().
		Str("network", listener.Addr().Network()).
		Str("address", listener.Addr().String()).
		Bool("tls", s.usesTLS()).
		Msg("server listening")
	return nil
}

// Stop 在配置的超时内停止 HTTP、Gateway 连接和 gRPC 服务。
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.config.gracefulTimeout)
	defer cancel()
	return s.stop(ctx)
}

func (s *Server) stop(ctx context.Context) error {
	s.mu.Lock()
	switch s.state {
	case serverStateStopped:
		err := s.stopErr
		s.mu.Unlock()
		return err
	case serverStateStopping:
		done := s.stopDone
		s.mu.Unlock()
		select {
		case <-done:
			s.mu.Lock()
			err := s.stopErr
			s.mu.Unlock()
			return err
		case <-ctx.Done():
			return errx.Wrap(ctx.Err(), "wait for server stop")
		}
	default:
		s.state = serverStateStopping
	}
	gatewayCancel := s.gatewayCancel
	s.mu.Unlock()

	err := s.shutdown(ctx, gatewayCancel)
	s.mu.Lock()
	s.stopErr = err
	s.state = serverStateStopped
	close(s.stopDone)
	s.mu.Unlock()
	return err
}

func (s *Server) shutdown(ctx context.Context, gatewayCancel context.CancelFunc) error {
	var result error
	httpDone := make(chan error, 1)
	go func() {
		httpDone <- s.httpServer.Shutdown(ctx)
	}()
	grpcDone := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(grpcDone)
	}()

	if err := <-httpDone; err != nil {
		result = errx.Wrap(err, "shutdown http server")
		if closeErr := s.httpServer.Close(); closeErr != nil {
			result = mergeErrors(result, errx.Wrap(closeErr, "close http server"))
		}
	}
	if gatewayCancel != nil {
		gatewayCancel()
	}

	select {
	case <-grpcDone:
	case <-ctx.Done():
		s.grpcServer.Stop()
		<-grpcDone
		result = mergeErrors(result, errx.Wrap(ctx.Err(), "gracefully stop grpc server"))
	}
	return result
}

func (s *Server) serve(name string, serve func() error) {
	err := serve()
	if s.isStopping() || isExpectedServeError(err) {
		return
	}
	if err == nil {
		err = errx.Newf("%s server stopped unexpectedly", name)
	} else {
		err = errx.Wrapf(err, "%s server", name)
	}
	lifex.Shutdown(err)
}

func (s *Server) listen() (net.Listener, error) {
	if s.config.listener != nil {
		return s.config.listener, nil
	}
	listener, err := net.Listen("tcp", s.config.address)
	if err != nil {
		return nil, errx.Wrapf(err, "listen on %s", s.config.address)
	}
	return listener, nil
}

func (s *Server) finishStart() {
	s.grpcServer.Stop()
	if err := s.grpcListener.Close(); err != nil && !errx.Is(err, net.ErrClosed) {
		logx.Error().Err(err).Msg("close grpc listener after server start failure")
	}
	s.state = serverStateStopped
	close(s.stopDone)
}

func (s *Server) isStopping() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == serverStateStopping || s.state == serverStateStopped
}

func isExpectedServeError(err error) bool {
	return errx.Is(err, http.ErrServerClosed) || errx.Is(err, grpc.ErrServerStopped) || errx.Is(err, net.ErrClosed)
}

func mergeErrors(current, next error) error {
	if current == nil {
		return next
	}
	logx.Error().Err(next).Msg("additional shutdown error")
	return current
}
