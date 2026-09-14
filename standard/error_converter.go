package standard

import (
	"context"
	"reflect"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"google.golang.org/grpc"
)

// ErrorConverter 将应用依赖返回的错误转换为统一响应错误。
type ErrorConverter interface {
	Convert(error) (RespError, bool)
}

// ErrorConvertFunc 将函数适配为 ErrorConverter。
type ErrorConvertFunc func(error) (RespError, bool)

// Convert 执行错误转换。
func (f ErrorConvertFunc) Convert(err error) (RespError, bool) {
	return f(err)
}

func unaryErrorConverterInterceptor(converters []ErrorConverter, bundle *i18n.Bundle) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		response, err := handler(ctx, req)
		return response, localizeResponseError(ctx, convertResponseError(err, converters), bundle)
	}
}

func streamErrorConverterInterceptor(converters []ErrorConverter, bundle *i18n.Bundle) grpc.StreamServerInterceptor {
	return func(
		srv any,
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		return localizeResponseError(stream.Context(), convertResponseError(handler(srv, stream), converters), bundle)
	}
}

func convertResponseError(err error, converters []ErrorConverter) error {
	if err == nil {
		return nil
	}
	for _, converter := range converters {
		if converted, ok := converter.Convert(err); ok {
			return converted
		}
	}
	return err
}

func isNilErrorConverter(converter ErrorConverter) bool {
	if converter == nil {
		return true
	}
	value := reflect.ValueOf(converter)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
