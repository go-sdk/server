package standard

import (
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
)

// ServeMux 是 runtime.ServeMux 的类型别名，用于 Gateway 路由注册。
type ServeMux = runtime.ServeMux

// ServeMuxOption 是 runtime.ServeMuxOption 的类型别名，用于 Gateway 配置。
type ServeMuxOption = runtime.ServeMuxOption

// ServiceRegistrar 是 grpc.ServiceRegistrar 的类型别名，用于 gRPC 服务注册。
type ServiceRegistrar = grpc.ServiceRegistrar

// ClientConn 是 grpc.ClientConn 的类型别名，表示 gRPC 客户端连接。
type ClientConn = grpc.ClientConn

// ClientConnInterface 是 grpc.ClientConnInterface 的类型别名，用于生成的 gRPC 客户端。
type ClientConnInterface = grpc.ClientConnInterface

// DialOption 是 grpc.DialOption 的类型别名，用于 gRPC 连接配置。
type DialOption = grpc.DialOption

// CallOption 是 grpc.CallOption 的类型别名，用于单次 RPC 调用配置。
type CallOption = grpc.CallOption

// ServerOption 是 grpc.ServerOption 的类型别名，用于 gRPC Server 配置。
type ServerOption = grpc.ServerOption
