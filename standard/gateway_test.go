package standard

import (
	"context"
	"net"
	"net/http/httptest"
	"testing"
)

type testListener struct{ address net.Addr }

func (l testListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }

func (l testListener) Close() error { return nil }

func (l testListener) Addr() net.Addr { return l.address }

type testAddress struct {
	network string
	address string
}

func (a testAddress) Network() string { return a.network }

func (a testAddress) String() string { return a.address }

func TestGatewayEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		server   *Server
		listener net.Listener
		want     string
		wantErr  bool
	}{
		{
			name:     "configured endpoint",
			server:   &Server{config: config{gatewayEndpoint: "gateway:8080"}},
			listener: testListener{address: testAddress{network: "bufconn", address: "bufconn"}},
			want:     "gateway:8080",
		},
		{
			name:     "ipv4 unspecified address",
			server:   &Server{},
			listener: testListener{address: testAddress{network: "tcp", address: "0.0.0.0:8080"}},
			want:     "127.0.0.1:8080",
		},
		{
			name:     "ipv6 unspecified address",
			server:   &Server{},
			listener: testListener{address: testAddress{network: "tcp6", address: "[::]:8080"}},
			want:     "[::1]:8080",
		},
		{
			name:     "non tcp listener",
			server:   &Server{},
			listener: testListener{address: testAddress{network: "bufconn", address: "bufconn"}},
			wantErr:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			endpoint, err := test.server.gatewayEndpoint(test.listener)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected gateway endpoint error")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve gateway endpoint: %v", err)
			}
			if endpoint != test.want {
				t.Fatalf("unexpected endpoint: %q", endpoint)
			}
		})
	}
}

func TestGatewayRequestMetadataForwardsAcceptLanguage(t *testing.T) {
	ctx := NewContext(context.Background(),
		TraceIDKey, "trace-1",
		AcceptLanguageKey, "zh-CN,en;q=0.8",
	)
	request := httptest.NewRequest("GET", "/users", nil)
	values := gatewayRequestMetadata(ctx, request)

	assertMetadataValue(t, values, acceptLanguageMetadataKey, "zh-CN,en;q=0.8")
}
