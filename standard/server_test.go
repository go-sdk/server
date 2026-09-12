package standard

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	grpchealthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
)

const inspectMethod = "/standard.test.ContextService/Inspect"

type contextService interface {
	Inspect(context.Context, *emptypb.Empty) (*structpb.Struct, error)
}

type contextServiceImpl struct{}

func (contextServiceImpl) Inspect(ctx context.Context, _ *emptypb.Empty) (*structpb.Struct, error) {
	requestContext := FromContext(ctx)
	return structpb.NewStruct(map[string]any{
		"request_id": requestContext.RequestID(),
		"depth":      requestContext.Depth(),
		"subject":    requestContext.JWT()["sub"],
	})
}

func contextServiceHandler(service any, ctx context.Context, decode func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	request := &emptypb.Empty{}
	if err := decode(request); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return service.(contextService).Inspect(ctx, request)
	}
	info := &grpc.UnaryServerInfo{Server: service, FullMethod: inspectMethod}
	handler := func(ctx context.Context, request any) (any, error) {
		return service.(contextService).Inspect(ctx, request.(*emptypb.Empty))
	}
	return interceptor(ctx, request, info, handler)
}

func TestServerWithBufconn(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	secret := []byte("test-secret")
	server, err := New(
		WithListener(listener),
		WithJWTSecret(secret),
		WithGRPCRegister(func(registrar grpc.ServiceRegistrar) {
			registrar.RegisterService(&grpc.ServiceDesc{
				ServiceName: "standard.test.ContextService",
				HandlerType: (*contextService)(nil),
				Methods: []grpc.MethodDesc{{
					MethodName: "Inspect",
					Handler:    contextServiceHandler,
				}},
			}, contextServiceImpl{})
		}),
	)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		if err := server.Stop(); err != nil {
			t.Errorf("stop server: %v", err)
		}
	})

	conn, err := NewClient(
		"passthrough:///bufconn",
		WithClientDialOptions(grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		})),
	)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close client: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	healthResponse, err := grpchealthv1.NewHealthClient(conn).Check(ctx, &grpchealthv1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("check health: %v", err)
	}
	if healthResponse.GetStatus() != grpchealthv1.HealthCheckResponse_SERVING {
		t.Fatalf("unexpected health status: %s", healthResponse.GetStatus())
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "user-1"})
	signedToken, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	authorization := "Bearer " + signedToken
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(
		requestIDMetadataKey, "request-1",
		depthMetadataKey, "2",
		"authorization", authorization,
	))
	ctx, _ = grpcRequestContext(ctx)
	ctx, err = contextWithJWT(ctx, authorization, secret)
	if err != nil {
		t.Fatalf("initialize source jwt context: %v", err)
	}
	response := &structpb.Struct{}
	if err := conn.Invoke(ctx, inspectMethod, &emptypb.Empty{}, response); err != nil {
		t.Fatalf("invoke context service: %v", err)
	}
	fields := response.AsMap()
	if fields["request_id"] != "request-1" {
		t.Fatalf("unexpected propagated request id: %v", fields["request_id"])
	}
	if fields["depth"] != float64(3) {
		t.Fatalf("unexpected propagated depth: %v", fields["depth"])
	}
	if fields["subject"] != "user-1" {
		t.Fatalf("unexpected propagated jwt subject: %v", fields["subject"])
	}
}

func TestWithCertificatePEM(t *testing.T) {
	certPEM, keyPEM := generateCertificatePEM(t)
	listener := bufconn.Listen(1 << 20)
	t.Cleanup(func() { _ = listener.Close() })
	server, err := New(
		WithListener(listener),
		WithCertificatePEM(certPEM, keyPEM),
	)
	if err != nil {
		t.Fatalf("create tls server: %v", err)
	}
	if !server.usesTLS() {
		t.Fatal("pem certificate must enable tls")
	}
	if server.httpServer.TLSConfig == nil || len(server.httpServer.TLSConfig.Certificates) != 1 {
		t.Fatal("pem certificate must be installed into the http tls config")
	}
	if err := server.Stop(); err != nil {
		t.Fatalf("stop unstarted server: %v", err)
	}
}

func TestClientTransportCredentials(t *testing.T) {
	plaintext, err := clientTransportCredentials(clientConfig{})
	if err != nil {
		t.Fatalf("create plaintext credentials: %v", err)
	}
	if plaintext.Info().SecurityProtocol != insecure.NewCredentials().Info().SecurityProtocol {
		t.Fatalf("unexpected plaintext protocol: %q", plaintext.Info().SecurityProtocol)
	}
	tlsCredentials, err := clientTransportCredentials(clientConfig{tlsConfig: &tls.Config{MinVersion: tls.VersionTLS12}})
	if err != nil {
		t.Fatalf("create tls credentials: %v", err)
	}
	if tlsCredentials.Info().SecurityProtocol != "tls" {
		t.Fatalf("unexpected tls protocol: %q", tlsCredentials.Info().SecurityProtocol)
	}
}

func TestCloseClientIsIdempotent(t *testing.T) {
	conn, err := grpc.NewClient("passthrough:///unused", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := closeClient(conn); err != nil {
		t.Fatalf("close client: %v", err)
	}
	if err := closeClient(conn); err != nil {
		t.Fatalf("close client again: %v", err)
	}
}

func generateCertificatePEM(t *testing.T) ([]byte, []byte) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certificate, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER})
}
