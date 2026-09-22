package standard

import (
	"context"
	"strings"

	grpcmiddleware "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	serveroptions "github.com/go-sdk/server/options"
)

const (
	healthServiceName            = "grpc.health.v1.Health"
	reflectionV1ServiceName      = "grpc.reflection.v1.ServerReflection"
	reflectionV1AlphaServiceName = "grpc.reflection.v1alpha.ServerReflection"
)

func methodOption(fullMethod string) *serveroptions.MethodOptions {
	serviceName, methodName, ok := splitFullMethod(fullMethod)
	if !ok {
		return nil
	}
	descriptor, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(serviceName))
	if err != nil {
		return nil
	}
	service, ok := descriptor.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil
	}
	method := service.Methods().ByName(protoreflect.Name(methodName))
	if method == nil {
		return nil
	}
	methodOptions, ok := method.Options().(*descriptorpb.MethodOptions)
	if !ok || !proto.HasExtension(methodOptions, serveroptions.E_Method) {
		return nil
	}
	option, _ := proto.GetExtension(methodOptions, serveroptions.E_Method).(*serveroptions.MethodOptions)
	return option
}

func splitFullMethod(fullMethod string) (string, string, bool) {
	serviceName, methodName, ok := strings.Cut(strings.TrimPrefix(fullMethod, "/"), "/")
	return serviceName, methodName, ok && serviceName != "" && methodName != ""
}

func isHealthMethod(fullMethod string) bool {
	serviceName, _, ok := splitFullMethod(fullMethod)
	return ok && serviceName == healthServiceName
}

func isSystemMethod(fullMethod string) bool {
	serviceName, _, ok := splitFullMethod(fullMethod)
	if !ok {
		return false
	}
	switch serviceName {
	case healthServiceName, reflectionV1ServiceName, reflectionV1AlphaServiceName:
		return true
	default:
		return false
	}
}

func shouldSkipMethodLog(fullMethod string) bool {
	option := methodOption(fullMethod)
	return option != nil && option.GetSkipLog()
}

func shouldLogMethod(_ context.Context, callMeta grpcmiddleware.CallMeta) bool {
	fullMethod := "/" + callMeta.Service + "/" + callMeta.Method
	return !shouldSkipMethodLog(fullMethod)
}

func shouldSkipMethodAuth(fullMethod string) bool {
	if isHealthMethod(fullMethod) {
		return true
	}
	option := methodOption(fullMethod)
	return option != nil && option.GetSkipAuth()
}
