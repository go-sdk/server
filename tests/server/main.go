package main

import (
	"os"

	"github.com/go-sdk/core/lifex"
	"github.com/go-sdk/core/logx"
	"google.golang.org/grpc"

	"github.com/go-sdk/server/standard"
	bizv1 "github.com/go-sdk/server/tests/pb/biz/v1"
	corev1 "github.com/go-sdk/server/tests/pb/core/v1"
)

func main() {
	_, err := standard.New(
		standard.WithAddress(":8080"),
		standard.WithReflection(),
		standard.WithGRPCRegister(func(registrar grpc.ServiceRegistrar) {
			corev1.RegisterUserServiceServer(registrar, &userService{})
			bizv1.RegisterBillServiceServer(registrar, &billService{})
		}),
		standard.WithGatewayRegister(
			corev1.RegisterUserServiceHandlerFromEndpoint,
			bizv1.RegisterBillServiceHandlerFromEndpoint,
		),
	)
	if err != nil {
		logx.Fatal().Err(err).Msg("failed to create server")
	}

	if err = lifex.Init(); err != nil {
		lifex.Shutdown(err)
	}
	if err = lifex.Wait(); err != nil {
		logx.Error().Err(err).Msg("server stopped unexpectedly")
		os.Exit(1)
	}
}
