package standard

import (
	"maps"
	"reflect"
	"strconv"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"
	"google.golang.org/protobuf/reflect/protoreflect"

	serveroptions "github.com/go-sdk/server/options"
)

// RespError 描述可同时供 gRPC 和 Gateway 返回的业务错误。
type RespError struct {
	code    codes.Code
	message string
	domain  string
	reason  string
	details []proto.Message
	data    map[string]any

	defaultMessage string
	hasErrorCode   bool

	httpStatus int
}

var (
	// ErrInternal 表示未向调用方暴露内部实现细节的服务端错误。
	ErrInternal = NewError(codes.Internal, "internal server error")
	// ErrInvalidParam 表示请求参数不合法。
	ErrInvalidParam = NewError(codes.InvalidArgument, "invalid parameter")
	// ErrUnauthenticated 表示调用方尚未通过身份认证。
	ErrUnauthenticated = NewError(codes.Unauthenticated, "unauthenticated")
	// ErrNotFound 表示请求的资源不存在。
	ErrNotFound = NewError(codes.NotFound, "resource not found")
	// ErrPermissionDenied 表示调用方无权执行当前操作。
	ErrPermissionDenied = NewError(codes.PermissionDenied, "permission denied")
	// ErrAlreadyExists 表示待创建的资源已经存在。
	ErrAlreadyExists = NewError(codes.AlreadyExists, "resource already exists")
	// ErrResourceExhausted 表示请求超过服务允许的资源限制。
	ErrResourceExhausted = NewError(codes.ResourceExhausted, "resource exhausted")
	// ErrFailedPrecondition 表示当前系统状态不满足操作前置条件。
	ErrFailedPrecondition = NewError(codes.FailedPrecondition, "failed precondition")
	// ErrAborted 表示操作因并发冲突等原因被中止。
	ErrAborted = NewError(codes.Aborted, "operation aborted")
	// ErrUnavailable 表示服务暂时不可用。
	ErrUnavailable = NewError(codes.Unavailable, "service unavailable")
)

// NewError 创建指定 gRPC Code 和消息的响应错误。
func NewError(code codes.Code, message string) RespError {
	return RespError{code: code, message: message}
}

// WithMessage 设置返回给调用方的错误消息。
func (e RespError) WithMessage(message string) RespError {
	e.message = message
	return e
}

// WithHTTPStatus 设置通过 HandlePath 返回错误时使用的 HTTP 状态码。
func (e RespError) WithHTTPStatus(statusCode int) RespError {
	e.httpStatus = statusCode
	return e
}

// WithDomainReason 设置由业务定义的错误码和 i18n 信息。
func (e RespError) WithDomainReason(domain, reason string) RespError {
	e.domain = domain
	e.reason = reason
	e.defaultMessage = ""
	e.hasErrorCode = false
	return e
}

// WithErrorCode 从枚举选项读取业务错误码、英文默认文案和额外 HTTP 状态码。
func (e RespError) WithErrorCode(errorCode protoreflect.Enum) RespError {
	if errorCode == nil {
		return e
	}
	number := errorCode.Number()
	e.domain = strconv.FormatInt(int64(number), 10)
	e.reason = e.domain
	e.defaultMessage = ""
	e.hasErrorCode = true

	valueDescriptor := errorCode.Descriptor().Values().ByNumber(number)
	if valueDescriptor == nil {
		return e
	}
	e.reason = string(valueDescriptor.Name())
	valueOptions := valueDescriptor.Options()
	if !proto.HasExtension(valueOptions, serveroptions.E_EnumValue) {
		return e
	}
	options, ok := proto.GetExtension(valueOptions, serveroptions.E_EnumValue).(*serveroptions.EnumValueOptions)
	if !ok || options == nil {
		return e
	}
	e.defaultMessage = options.GetMessage()
	if options.GetHttpStatus() > 0 {
		e.httpStatus = int(options.GetHttpStatus())
	}
	return e
}

// WithData 设置错误文案模板使用的变量，并复制数据以保持错误模板不可变。
func (e RespError) WithData(data map[string]any) RespError {
	e.data = maps.Clone(data)
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
