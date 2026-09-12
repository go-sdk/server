package standard

import (
	"context"
	"reflect"

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

func unaryErrorConverterInterceptor(converters []ErrorConverter) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		response, err := handler(ctx, req)
		return response, convertResponseError(err, converters)
	}
}

func streamErrorConverterInterceptor(converters []ErrorConverter) grpc.StreamServerInterceptor {
	return func(
		srv any,
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		return convertResponseError(handler(srv, stream), converters)
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
