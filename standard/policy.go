package standard

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/go-sdk/core/logx"
	"google.golang.org/grpc"
)

// PermissionFunc 根据方法声明的权限执行业务校验。
type PermissionFunc func(context.Context, string, []string) error

// AuditFunc 将调用结果交给业务层记录。
type AuditFunc func(context.Context, AuditInvocation) error

// AuditInvocation 是一次需要审计的 RPC 调用，请求和响应已按字段选项脱敏。
type AuditInvocation struct {
	Method   string
	Kind     string
	Request  any
	Response any
	Err      error
	Duration time.Duration
}

func unaryPermissionInterceptor(invoke PermissionFunc) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := invokePermission(ctx, info.FullMethod, invoke); err != nil {
			return nil, err
		}
		return handler(ctx, request)
	}
}

func streamPermissionInterceptor(invoke PermissionFunc) grpc.StreamServerInterceptor {
	return func(server any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := invokePermission(stream.Context(), info.FullMethod, invoke); err != nil {
			return err
		}
		return handler(server, stream)
	}
}

func invokePermission(ctx context.Context, fullMethod string, invoke PermissionFunc) error {
	if isSystemMethod(fullMethod) {
		return nil
	}
	option := methodOption(fullMethod)
	if option == nil {
		return ErrInternal.WithMessage("resolve method options")
	}
	if option.GetSkipAuth() || len(option.GetPermissions()) == 0 {
		return nil
	}
	return invoke(ctx, fullMethod, slices.Clone(option.GetPermissions()))
}

func unaryAuditInterceptor(invoke AuditFunc) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		option := methodOption(info.FullMethod)
		if option == nil {
			return handler(ctx, request)
		}
		kind := strings.TrimSpace(option.GetAuditKind())
		if kind == "" {
			return handler(ctx, request)
		}
		requestPayload, _ := logPayload(request, option.GetSkipLog())
		startedAt := time.Now()
		response, err := handler(ctx, request)
		responsePayload, _ := logPayload(response, option.GetSkipLog())
		invokeAudit(ctx, invoke, AuditInvocation{
			Method:   info.FullMethod,
			Kind:     kind,
			Request:  requestPayload,
			Response: responsePayload,
			Err:      err,
			Duration: time.Since(startedAt),
		})
		return response, err
	}
}

func streamAuditInterceptor(invoke AuditFunc) grpc.StreamServerInterceptor {
	return func(server any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		option := methodOption(info.FullMethod)
		if option == nil {
			return handler(server, stream)
		}
		kind := strings.TrimSpace(option.GetAuditKind())
		if kind == "" {
			return handler(server, stream)
		}
		startedAt := time.Now()
		err := handler(server, stream)
		invokeAudit(stream.Context(), invoke, AuditInvocation{
			Method:   info.FullMethod,
			Kind:     kind,
			Err:      err,
			Duration: time.Since(startedAt),
		})
		return err
	}
}

func invokeAudit(ctx context.Context, invoke AuditFunc, invocation AuditInvocation) {
	if err := invoke(ctx, invocation); err != nil {
		logx.Ctx(ctx).Error().Err(err).Str("grpc_method", invocation.Method).Msg("record audit")
	}
}
