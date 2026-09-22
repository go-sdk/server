package standard

import (
	"context"
	"testing"

	"google.golang.org/grpc"

	corev1 "github.com/go-sdk/server/tests/pb/core/v1"
)

func TestInvokePermissionUsesMethodOptions(t *testing.T) {
	called := false
	err := invokePermission(context.Background(), corev1.UserService_Create_FullMethodName, func(_ context.Context, method string, permissions []string) error {
		called = true
		if method != corev1.UserService_Create_FullMethodName {
			t.Fatalf("unexpected method: %s", method)
		}
		if len(permissions) != 1 || permissions[0] != "user.create" {
			t.Fatalf("unexpected permissions: %v", permissions)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("invoke permission: %v", err)
	}
	if !called {
		t.Fatal("permission func was not called")
	}
}

func TestInvokePermissionRequiresMethodOptions(t *testing.T) {
	err := invokePermission(context.Background(), "/unknown.Service/Call", func(context.Context, string, []string) error { return nil })
	if err == nil {
		t.Fatal("expected missing method options error")
	}
}

func TestUnaryAuditUsesKindAndRedactsPayload(t *testing.T) {
	var invocation AuditInvocation
	interceptor := unaryAuditInterceptor(func(_ context.Context, value AuditInvocation) error {
		invocation = value
		return nil
	})
	request := &corev1.CreateUserReq{Name: "tester", Email: "tester@example.com"}
	response, err := interceptor(context.Background(), request, &grpc.UnaryServerInfo{
		FullMethod: corev1.UserService_Create_FullMethodName,
	}, func(context.Context, any) (any, error) {
		return request, nil
	})
	if err != nil {
		t.Fatalf("invoke audited method: %v", err)
	}
	if invocation.Kind != "user.create" {
		t.Fatalf("unexpected audit kind: %s", invocation.Kind)
	}
	if response != request {
		t.Fatal("unexpected response")
	}
	fields, ok := invocation.Request.(map[string]any)
	if !ok {
		t.Fatalf("unexpected audit request: %T", invocation.Request)
	}
	if fields["email"] != redactedValue {
		t.Fatalf("sensitive audit field was not redacted: %v", fields["email"])
	}
}
