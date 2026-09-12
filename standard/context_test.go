package standard

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestNewContext(t *testing.T) {
	ctx := NewContext(context.Background(),
		RequestIDKey, "request-1",
		DepthKey, 2,
		ClientIPKey, "127.0.0.1",
		ContentTypeKey, "application/grpc",
		UserAgentKey, "test-agent",
		JWTKey, jwt.MapClaims{"sub": "user-1"},
	)
	requestContext := FromContext(ctx)
	if requestContext.RequestID() != "request-1" {
		t.Fatalf("unexpected request id: %q", requestContext.RequestID())
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
	claims := requestContext.JWT()
	claims["sub"] = "changed"
	if requestContext.JWT()["sub"] != "user-1" {
		t.Fatal("jwt claims must be returned as a copy")
	}

	derived := NewContext(ctx, DepthKey, 3)
	if FromContext(derived).RequestID() != "request-1" || FromContext(derived).Depth() != 3 {
		t.Fatal("derived context must preserve existing values")
	}
}
