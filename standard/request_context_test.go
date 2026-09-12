package standard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestHTTPRequestContextMiddleware(t *testing.T) {
	var requestContext *Context
	handler := httpRequestContextMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		requestContext = FromContext(request.Context())
	}))
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.RemoteAddr = "192.0.2.1:1234"
	request.Header.Set(requestIDHeader, "request-1")
	request.Header.Set(depthHeader, "2")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "test-agent")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Header().Get(requestIDHeader) != "request-1" {
		t.Fatalf("unexpected response request id: %q", response.Header().Get(requestIDHeader))
	}
	if requestContext == nil {
		t.Fatal("request context was not initialized")
	}
	if requestContext.RequestID() != "request-1" || requestContext.Depth() != 2 {
		t.Fatalf("unexpected request context: request_id=%q depth=%d", requestContext.RequestID(), requestContext.Depth())
	}
	if requestContext.ClientIP() != "192.0.2.1" || requestContext.ContentType() != "application/json" || requestContext.UserAgent() != "test-agent" {
		t.Fatal("request metadata was not initialized")
	}
}

func TestOutgoingRequestContext(t *testing.T) {
	ctx := metadata.AppendToOutgoingContext(context.Background(), "custom", "value")
	ctx = NewContext(ctx, RequestIDKey, "request-1", DepthKey, 2)
	ctx = contextWithAuthorization(ctx, "Bearer token")
	outgoing := outgoingRequestContext(ctx)
	values, ok := metadata.FromOutgoingContext(outgoing)
	if !ok {
		t.Fatal("outgoing metadata was not initialized")
	}
	assertMetadataValue(t, values, "custom", "value")
	assertMetadataValue(t, values, requestIDMetadataKey, "request-1")
	assertMetadataValue(t, values, depthMetadataKey, "3")
	assertMetadataValue(t, values, "authorization", "Bearer token")
}

func TestHTTPRecoveryMiddleware(t *testing.T) {
	handler := httpRecoveryMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("test panic")
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d", response.Code)
	}
}

func assertMetadataValue(t *testing.T, values metadata.MD, key, want string) {
	t.Helper()
	items := values.Get(key)
	if len(items) != 1 || items[0] != want {
		t.Fatalf("unexpected metadata %s: %v", key, items)
	}
}
