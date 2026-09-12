package main

import (
	"context"

	"github.com/go-sdk/core/seq"

	bizv1 "github.com/go-sdk/server/tests/pb/biz/v1"
	"github.com/go-sdk/server/tests/pb/common"
	corev1 "github.com/go-sdk/server/tests/pb/core/v1"
)

type userService struct {
	corev1.UnimplementedUserServiceServer
}

func (s *userService) Health(_ context.Context, _ *common.Empty) (*common.Empty, error) {
	return &common.Empty{}, nil
}

func (s *userService) Create(_ context.Context, _ *corev1.CreateUserReq) (*common.Id, error) {
	return &common.Id{Id: seq.NextID()}, nil
}

type billService struct {
	bizv1.UnimplementedBillServiceServer
}

func (s *billService) List(_ context.Context, req *bizv1.ListBillReq) (*bizv1.ListBillResp, error) {
	return &bizv1.ListBillResp{Paging: req.Paging}, nil
}
