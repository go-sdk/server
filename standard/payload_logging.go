package standard

import (
	"context"
	"encoding/base64"
	"fmt"
	"reflect"
	"time"

	"github.com/go-sdk/core/logx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/anypb"

	serveroptions "github.com/go-sdk/server/options"
)

const redactedValue = "***"

func unaryPayloadLoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if isHealthMethod(info.FullMethod) {
			return handler(ctx, req)
		}
		redactPayload := shouldSkipMethodLog(info.FullMethod)
		logGRPCRequest(ctx, "server", info.FullMethod, req, redactPayload)
		startedAt := time.Now()
		resp, err := handler(ctx, req)
		logGRPCResponse(ctx, "server", info.FullMethod, resp, err, time.Since(startedAt), redactPayload)
		return resp, err
	}
}

func streamPayloadLoggingInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if isHealthMethod(info.FullMethod) {
			return handler(srv, stream)
		}
		return handler(srv, &payloadLoggingServerStream{
			ServerStream:  stream,
			ctx:           stream.Context(),
			method:        info.FullMethod,
			redactPayload: shouldSkipMethodLog(info.FullMethod),
		})
	}
}

type payloadLoggingServerStream struct {
	grpc.ServerStream
	ctx           context.Context
	method        string
	redactPayload bool
}

func (s *payloadLoggingServerStream) Context() context.Context { return s.ctx }

func (s *payloadLoggingServerStream) RecvMsg(message any) error {
	err := s.ServerStream.RecvMsg(message)
	if err == nil {
		logGRPCRequest(s.ctx, "server", s.method, message, s.redactPayload)
	}
	return err
}

func (s *payloadLoggingServerStream) SendMsg(message any) error {
	startedAt := time.Now()
	err := s.ServerStream.SendMsg(message)
	logGRPCResponse(s.ctx, "server", s.method, message, err, time.Since(startedAt), s.redactPayload)
	return err
}

func logGRPCRequest(ctx context.Context, side, method string, payload any, redact bool) {
	content, size := logPayload(payload, redact)
	requestContext := FromContext(ctx)
	logx.Ctx(ctx).Info().
		Str("grpc_side", side).
		Str("grpc_method", method).
		Str("client_ip", requestContext.ClientIP()).
		Str("content_type", requestContext.ContentType()).
		Str("user_agent", requestContext.UserAgent()).
		Int("content_length", size).
		Interface("payload", content).
		Msg("grpc request")
}

func logGRPCResponse(ctx context.Context, side, method string, payload any, err error, duration time.Duration, redact bool) {
	content, size := logPayload(payload, redact)
	logx.Ctx(ctx).Info().
		Str("grpc_side", side).
		Str("grpc_method", method).
		Str("status_code", status.Code(err).String()).
		Int("content_length", size).
		Dur("duration", duration).
		Interface("payload", content).
		Msg("grpc response")
}

func logPayload(payload any, redact bool) (any, int) {
	message, ok := payload.(proto.Message)
	if !ok || isNilProtoMessage(message) {
		if redact {
			return redactedValue, 0
		}
		return nil, 0
	}
	size := proto.Size(message)
	if redact {
		return redactedValue, size
	}
	return logMessage(message.ProtoReflect()), size
}

func isNilProtoMessage(message proto.Message) bool {
	if message == nil {
		return true
	}
	value := reflect.ValueOf(message)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

func logMessage(message protoreflect.Message) map[string]any {
	if anyMessage, ok := message.Interface().(*anypb.Any); ok {
		return logAny(anyMessage)
	}
	result := map[string]any{}
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		name := field.JSONName()
		if isSensitiveField(field) {
			result[name] = redactedValue
			return true
		}
		switch {
		case field.IsMap():
			result[name] = logMap(field.MapValue(), value.Map())
		case field.IsList():
			result[name] = logList(field, value.List())
		default:
			result[name] = logValue(field, value)
		}
		return true
	})
	return result
}

func logAny(message *anypb.Any) map[string]any {
	result := map[string]any{"@type": message.GetTypeUrl()}
	embedded, err := message.UnmarshalNew()
	if err != nil {
		result["value"] = redactedValue
		return result
	}
	result["value"] = logMessage(embedded.ProtoReflect())
	return result
}

func logMap(valueField protoreflect.FieldDescriptor, values protoreflect.Map) map[string]any {
	result := map[string]any{}
	values.Range(func(key protoreflect.MapKey, value protoreflect.Value) bool {
		result[fmt.Sprint(key.Interface())] = logValue(valueField, value)
		return true
	})
	return result
}

func logList(field protoreflect.FieldDescriptor, values protoreflect.List) []any {
	result := make([]any, 0, values.Len())
	for i := 0; i < values.Len(); i++ {
		result = append(result, logValue(field, values.Get(i)))
	}
	return result
}

func logValue(field protoreflect.FieldDescriptor, value protoreflect.Value) any {
	switch field.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return logMessage(value.Message())
	case protoreflect.EnumKind:
		enumValue := field.Enum().Values().ByNumber(value.Enum())
		if enumValue == nil {
			return value.Enum()
		}
		return enumValue.Name()
	case protoreflect.BytesKind:
		return base64.StdEncoding.EncodeToString(value.Bytes())
	default:
		return value.Interface()
	}
}

func isSensitiveField(field protoreflect.FieldDescriptor) bool {
	fieldOptions, ok := field.Options().(*descriptorpb.FieldOptions)
	if !ok || !proto.HasExtension(fieldOptions, serveroptions.E_Field) {
		return false
	}
	option, ok := proto.GetExtension(fieldOptions, serveroptions.E_Field).(*serveroptions.FieldOptions)
	return ok && option.GetSensitive()
}
