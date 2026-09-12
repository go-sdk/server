package standard

import (
	"testing"

	corev1 "github.com/go-sdk/server/tests/pb/core/v1"
)

func TestLogPayloadRedactsSensitiveFields(t *testing.T) {
	payload, size := logPayload(&corev1.CreateUserReq{
		Name:  "tester",
		Email: "tester@example.com",
	}, false)
	if size == 0 {
		t.Fatal("payload size must be recorded")
	}
	fields, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("unexpected payload type: %T", payload)
	}
	if fields["name"] != "tester" {
		t.Fatalf("unexpected name: %v", fields["name"])
	}
	if fields["email"] != redactedValue {
		t.Fatalf("sensitive email was not redacted: %v", fields["email"])
	}
}

func TestMethodOptions(t *testing.T) {
	if !shouldSkipMethodAuth(corev1.UserService_Health_FullMethodName) {
		t.Fatal("health method must skip auth")
	}
	if !shouldSkipMethodLog(corev1.UserService_Health_FullMethodName) {
		t.Fatal("health method must skip payload logging")
	}
	if shouldSkipMethodAuth(corev1.UserService_Create_FullMethodName) {
		t.Fatal("create method must require auth")
	}
}
