// Package testserver 提供基于 bufconn 的标准 gRPC Server 测试环境。
package testserver

import (
	"context"
	"net"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	"github.com/go-sdk/server/standard"
)

const bufferSize = 1 << 20

// Server 持有测试期间运行的标准 Server 和指向该服务的 ClientConn。
type Server struct {
	t        testing.TB
	server   *standard.Server
	listener *bufconn.Listener
	conn     *grpc.ClientConn
	close    sync.Once
}

// New 启动内存 gRPC Server，并在测试结束时自动释放资源。
func New(t testing.TB, opts ...standard.Option) *Server {
	t.Helper()
	listener := bufconn.Listen(bufferSize)
	opts = append(opts, standard.WithListener(listener))
	grpcServer, err := standard.New(opts...)
	if err != nil {
		_ = listener.Close()
		t.Fatalf("create test server: %v", err)
	}
	if err := grpcServer.Start(); err != nil {
		_ = listener.Close()
		t.Fatalf("start test server: %v", err)
	}
	conn, err := standard.NewClient(
		"passthrough:///bufconn",
		standard.WithClientDialOptions(grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		})),
	)
	if err != nil {
		_ = grpcServer.Stop()
		_ = listener.Close()
		t.Fatalf("create test client: %v", err)
	}
	server := &Server{t: t, server: grpcServer, listener: listener, conn: conn}
	t.Cleanup(server.Close)
	return server
}

// Conn 返回连接到测试 Server 的 gRPC ClientConn。
func (s *Server) Conn() *grpc.ClientConn { return s.conn }

// Close 释放 ClientConn、Server 和 bufconn Listener，支持重复调用。
func (s *Server) Close() {
	s.t.Helper()
	s.close.Do(func() {
		if err := s.conn.Close(); err != nil {
			s.t.Errorf("close test client: %v", err)
		}
		if err := s.server.Stop(); err != nil {
			s.t.Errorf("stop test server: %v", err)
		}
		if err := s.listener.Close(); err != nil {
			s.t.Errorf("close test listener: %v", err)
		}
	})
}
