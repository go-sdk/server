package main

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/go-sdk/server/common"
	"github.com/go-sdk/server/standard"
	"github.com/go-sdk/server/standard/testserver"
	bizv1 "github.com/go-sdk/server/tests/pb/biz/v1"
	corev1 "github.com/go-sdk/server/tests/pb/core/v1"
)

var testJWTSecret = []byte("test-secret")

func newServiceTestServer(t *testing.T) *testserver.Server {
	t.Helper()
	return testserver.New(t,
		standard.WithJWTSecret(testJWTSecret),
		standard.WithGRPCRegister(func(registrar grpc.ServiceRegistrar) {
			corev1.RegisterUserServiceServer(registrar, &userService{})
			bizv1.RegisterBillServiceServer(registrar, &billService{})
		}),
	)
}

func authenticatedContext(t *testing.T) context.Context {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "user-1"})
	signedToken, err := token.SignedString(testJWTSecret)
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+signedToken)
}

func TestUserService(t *testing.T) {
	server := newServiceTestServer(t)
	client := corev1.NewUserServiceClient(server.Conn())

	t.Run("health skips auth", func(t *testing.T) {
		if _, err := client.Health(context.Background(), &common.Empty{}); err != nil {
			t.Fatalf("check health: %v", err)
		}
	})

	t.Run("create requires auth", func(t *testing.T) {
		_, err := client.Create(context.Background(), &corev1.CreateUserReq{Name: "tester", Email: "tester@example.com"})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("unexpected status: %s", status.Code(err))
		}
	})

	t.Run("create validates request", func(t *testing.T) {
		_, err := client.Create(authenticatedContext(t), &corev1.CreateUserReq{})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("unexpected status: %s", status.Code(err))
		}
	})

	t.Run("create returns id", func(t *testing.T) {
		response, err := client.Create(authenticatedContext(t), &corev1.CreateUserReq{
			Name:  "tester",
			Email: "tester@example.com",
		})
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		if response.GetId() == "" {
			t.Fatal("created user id must not be empty")
		}
	})
}

func TestBillService(t *testing.T) {
	server := newServiceTestServer(t)
	client := bizv1.NewBillServiceClient(server.Conn())
	request := &bizv1.ListBillReq{Paging: common.NewPaging(2, 20)}
	response, err := client.List(authenticatedContext(t), request)
	if err != nil {
		t.Fatalf("list bills: %v", err)
	}
	if response.GetPaging() == request.GetPaging() {
		t.Fatal("response paging must not reuse request")
	}
	if expected := request.GetPaging().WithTotal(0); !proto.Equal(response.GetPaging(), expected) {
		t.Fatalf("unexpected paging: %v", response.GetPaging())
	}
}
