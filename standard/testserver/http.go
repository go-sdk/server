package testserver

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-sdk/server/standard"
)

const httpClientTimeout = 5 * time.Second

// HTTPServer 持有测试期间运行的标准 Server 和指向该服务的 HTTP Client。
type HTTPServer struct {
	t        testing.TB
	server   *standard.Server
	listener net.Listener
	client   *http.Client
	baseURL  string
	close    sync.Once
}

// NewHTTP 在本地回环地址启动额外 HTTP 接口，并在测试结束时自动释放资源。
func NewHTTP(t testing.TB, method string, path string, handler standard.HandlerFunc, opts ...standard.Option) *HTTPServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for test server: %v", err)
	}
	opts = append(opts, standard.WithListener(listener))
	httpServer, err := standard.New(opts...)
	if err != nil {
		_ = listener.Close()
		t.Fatalf("create test server: %v", err)
	}
	if err = httpServer.HandlePath(method, path, handler); err != nil {
		_ = httpServer.Stop()
		_ = listener.Close()
		t.Fatalf("register test http route: %v", err)
	}
	if err = httpServer.Start(); err != nil {
		_ = listener.Close()
		t.Fatalf("start test server: %v", err)
	}
	server := &HTTPServer{
		t:        t,
		server:   httpServer,
		listener: listener,
		client:   &http.Client{Timeout: httpClientTimeout},
		baseURL:  "http://" + listener.Addr().String(),
	}
	t.Cleanup(server.Close)
	return server
}

// Client 返回连接到测试 Server 的 HTTP Client。
func (s *HTTPServer) Client() *http.Client { return s.client }

// URL 返回测试 Server 上指定路径的完整 URL。
func (s *HTTPServer) URL(path string) string {
	return s.baseURL + "/" + strings.TrimPrefix(path, "/")
}

// Close 释放 HTTP Client、Server 和 Listener，支持重复调用。
func (s *HTTPServer) Close() {
	s.t.Helper()
	s.close.Do(func() {
		s.client.CloseIdleConnections()
		if err := s.server.Stop(); err != nil {
			s.t.Errorf("stop test server: %v", err)
		}
		if err := s.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			s.t.Errorf("close test listener: %v", err)
		}
	})
}
