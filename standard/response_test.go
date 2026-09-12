package standard

import (
	"context"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestGatewayResponseRewriterWrapsSuccessData(t *testing.T) {
	message, err := structpb.NewStruct(map[string]any{"name": "alice"})
	if err != nil {
		t.Fatalf("create response: %v", err)
	}
	rewritten, err := gatewayResponseRewriter(context.Background(), message)
	if err != nil {
		t.Fatalf("rewrite response: %v", err)
	}
	encoded, err := (&runtime.JSONPb{}).Marshal(rewritten)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if string(encoded) != `{"data":{"name":"alice"}}` {
		t.Fatalf("unexpected response: %s", encoded)
	}
}

func TestRespErrorIsImmutable(t *testing.T) {
	first := ErrInvalidParam.WithDomainReason("USER_1001", "user.name.required")
	second := ErrInvalidParam.WithDomainReason("USER_1002", "user.email.required")

	firstStatus := status.Convert(first)
	secondStatus := status.Convert(second)
	if firstStatus.Code() != codes.InvalidArgument || secondStatus.Code() != codes.InvalidArgument {
		t.Fatal("invalid parameter helper must use InvalidArgument")
	}
	firstInfo := firstStatus.Details()[0].(*errdetails.ErrorInfo)
	secondInfo := secondStatus.Details()[0].(*errdetails.ErrorInfo)
	if firstInfo.GetDomain() != "USER_1001" || secondInfo.GetDomain() != "USER_1002" {
		t.Fatal("response error values must not share mutable state")
	}
	if len(status.Convert(ErrInvalidParam).Details()) != 0 {
		t.Fatal("built-in response error must remain unchanged")
	}
}

func TestGatewayResponseRewriterFormatsError(t *testing.T) {
	badRequest := &errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{{
		Field:       "name",
		Description: "required",
	}}}
	responseError := ErrInvalidParam.
		WithDomainReason("USER_1001", "user.name.required").
		WithDetails(badRequest)

	rewritten, err := gatewayResponseRewriter(context.Background(), responseError.GRPCStatus().Proto())
	if err != nil {
		t.Fatalf("rewrite error: %v", err)
	}
	encoded, err := (&runtime.JSONPb{}).Marshal(rewritten)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	want := `{"code":3,"message":"invalid parameter","domain":"USER_1001","reason":"user.name.required","details":[{"@type":"type.googleapis.com/google.rpc.BadRequest","fieldViolations":[{"field":"name","description":"required"}]}]}`
	if string(encoded) != want {
		t.Fatalf("unexpected error response:\n got: %s\nwant: %s", encoded, want)
	}
}

func TestGatewayResponseRewriterKeepsEmptyDomainReason(t *testing.T) {
	rewritten, err := gatewayResponseRewriter(
		context.Background(),
		status.New(codes.NotFound, "not found").Proto(),
	)
	if err != nil {
		t.Fatalf("rewrite error: %v", err)
	}
	encoded, err := (&runtime.JSONPb{}).Marshal(rewritten)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if string(encoded) != `{"code":5,"message":"not found","domain":"","reason":"","details":[]}` {
		t.Fatalf("unexpected error response: %s", encoded)
	}
}
