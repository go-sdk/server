package standard

import (
	"reflect"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"
)

// RespError 描述可同时供 gRPC 和 Gateway 返回的业务错误。
type RespError struct {
	code    codes.Code
	message string
	domain  string
	reason  string
	details []proto.Message
}

var (
	// ErrInternal 表示未向调用方暴露内部实现细节的服务端错误。
	ErrInternal = NewError(codes.Internal, "internal server error")
	// ErrInvalidParam 表示请求参数不合法。
	ErrInvalidParam = NewError(codes.InvalidArgument, "invalid parameter")
)

// NewError 创建指定 gRPC Code 和消息的响应错误。
func NewError(code codes.Code, message string) RespError {
	return RespError{code: code, message: message}
}

// WithDomainReason 设置由业务定义的错误码和 i18n 信息。
func (e RespError) WithDomainReason(domain, reason string) RespError {
	e.domain = domain
	e.reason = reason
	return e
}

// WithDetails 追加结构化错误详情。
func (e RespError) WithDetails(details ...proto.Message) RespError {
	cloned := append([]proto.Message(nil), e.details...)
	e.details = append(cloned, details...)
	return e
}

func (e RespError) Error() string {
	return e.message
}

// GRPCStatus 将响应错误转换为 gRPC Status。
func (e RespError) GRPCStatus() *status.Status {
	grpcStatus := status.New(e.code, e.message)
	details := make([]protoadapt.MessageV1, 0, len(e.details)+1)
	if e.domain != "" || e.reason != "" {
		details = append(details, protoadapt.MessageV1Of(&errdetails.ErrorInfo{
			Domain: e.domain,
			Reason: e.reason,
		}))
	}
	for _, detail := range e.details {
		if isNilMessage(detail) {
			continue
		}
		details = append(details, protoadapt.MessageV1Of(detail))
	}
	if len(details) == 0 {
		return grpcStatus
	}
	withDetails, err := grpcStatus.WithDetails(details...)
	if err != nil {
		return grpcStatus
	}
	return withDetails
}

func isNilMessage(message proto.Message) bool {
	if message == nil {
		return true
	}
	value := reflect.ValueOf(message)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
