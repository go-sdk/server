package standard

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestNewContext(t *testing.T) {
	ctx := NewContext(context.Background(),
		TraceIDKey, "trace-1",
		SpanIDKey, "span-1",
		DepthKey, 2,
		ClientIPKey, "127.0.0.1",
		ContentTypeKey, "application/grpc",
		UserAgentKey, "test-agent",
		AcceptLanguageKey, "zh-CN,en;q=0.8",
		JWTKey, jwt.MapClaims{"sub": "user-1"},
	)
	requestContext := FromContext(ctx)
	if requestContext.TraceID() != "trace-1" {
		t.Fatalf("unexpected trace id: %q", requestContext.TraceID())
	}
	if requestContext.SpanID() != "span-1" {
		t.Fatalf("unexpected span id: %q", requestContext.SpanID())
	}
	if requestContext.Depth() != 2 {
		t.Fatalf("unexpected depth: %d", requestContext.Depth())
	}
	if requestContext.ClientIP() != "127.0.0.1" {
		t.Fatalf("unexpected client ip: %q", requestContext.ClientIP())
	}
	if requestContext.ContentType() != "application/grpc" {
		t.Fatalf("unexpected content type: %q", requestContext.ContentType())
	}
	if requestContext.UserAgent() != "test-agent" {
		t.Fatalf("unexpected user agent: %q", requestContext.UserAgent())
	}
	if requestContext.AcceptLanguage() != "zh-CN,en;q=0.8" {
		t.Fatalf("unexpected accept language: %q", requestContext.AcceptLanguage())
	}
	claims := requestContext.JWT()
	claims["sub"] = "changed"
	if requestContext.JWT()["sub"] != "user-1" {
		t.Fatal("jwt claims must be returned as a copy")
	}

	derived := NewContext(ctx, DepthKey, 3)
	if FromContext(derived).TraceID() != "trace-1" || FromContext(derived).Depth() != 3 {
		t.Fatal("derived context must preserve existing values")
	}
}
