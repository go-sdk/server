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
	request.Header.Set(traceIDHeader, "trace-1")
	request.Header.Set(depthHeader, "2")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "test-agent")
	request.Header.Set("Accept-Language", "zh-CN,en;q=0.8")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Header().Get(traceIDHeader) != "trace-1" {
		t.Fatalf("unexpected response trace id: %q", response.Header().Get(traceIDHeader))
	}
	if response.Header().Get("X-Span-ID") != "" {
		t.Fatal("span id must not be exposed in the response")
	}
	if requestContext == nil {
		t.Fatal("request context was not initialized")
	}
	if requestContext.TraceID() != "trace-1" || requestContext.Depth() != 2 {
		t.Fatalf("unexpected request context: trace_id=%q depth=%d", requestContext.TraceID(), requestContext.Depth())
	}
	if requestContext.SpanID() == "" {
		t.Fatal("span id must be generated")
	}
	if requestContext.ClientIP() != "192.0.2.1" || requestContext.ContentType() != "application/json" || requestContext.UserAgent() != "test-agent" {
		t.Fatal("request metadata was not initialized")
	}
	if requestContext.AcceptLanguage() != "zh-CN,en;q=0.8" {
		t.Fatalf("unexpected accept language: %q", requestContext.AcceptLanguage())
	}
}

func TestOutgoingRequestContext(t *testing.T) {
	ctx := metadata.AppendToOutgoingContext(context.Background(), "custom", "value")
	ctx = NewContext(ctx, TraceIDKey, "trace-1", AcceptLanguageKey, "zh-CN", DepthKey, 2)
	ctx = contextWithAuthorization(ctx, "Bearer token")
	outgoing := outgoingRequestContext(ctx)
	values, ok := metadata.FromOutgoingContext(outgoing)
	if !ok {
		t.Fatal("outgoing metadata was not initialized")
	}
	assertMetadataValue(t, values, "custom", "value")
	assertMetadataValue(t, values, traceIDMetadataKey, "trace-1")
	assertMetadataValue(t, values, depthMetadataKey, "3")
	assertMetadataValue(t, values, "authorization", "Bearer token")
	assertMetadataValue(t, values, acceptLanguageMetadataKey, "zh-CN")
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
