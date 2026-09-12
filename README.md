# server

`server` 是个人使用的 Go 服务基础类库，模块路径为 `github.com/go-sdk/server`。项目使用 gmux 在同一个端口提供原生 gRPC 和 grpc-gateway HTTP API，通过 `core/lifex` 管理进程生命周期，并统一 Request ID、访问日志、Protovalidate、Recovery、TLS 和优雅停止行为。

## 环境要求

- Go 1.26 或更高版本

## 安装

```bash
go get github.com/go-sdk/server
```

## 服务模型

业务接口以 Protobuf gRPC 服务为唯一来源，通过 `google.api.http` 注解生成 HTTP API。Gateway 使用生成的 `Register*HandlerFromEndpoint` 回连同端口的真实 gRPC Server，因此 HTTP 和原生 gRPC 请求经过相同的 gRPC interceptor。

上传、下载或 Webhook 等不适合建模为 RPC 的接口，可以通过 `Server.HandlePath` 直接注册到内部 Gateway Mux。底层 `runtime.ServeMux`、`http.Server` 和 `grpc.Server` 不对外暴露。

## 创建服务

```go
userService := NewUserService()

server, err := standard.New(
	standard.WithAddress(":8080"),
	standard.WithGRPCRegister(
		func(registrar grpc.ServiceRegistrar) {
			corev1.RegisterUserServiceServer(registrar, userService)
		},
	),
	standard.WithGatewayRegister(
		corev1.RegisterUserServiceHandlerFromEndpoint,
	),
)
if err != nil {
	return err
}
```

多个服务可以在同一个 Option 中注册：

```go
standard.WithGRPCRegister(
	func(registrar grpc.ServiceRegistrar) {
		corev1.RegisterUserServiceServer(registrar, userService)
	},
	func(registrar grpc.ServiceRegistrar) {
		bizv1.RegisterBillServiceServer(registrar, billService)
	},
)

standard.WithGatewayRegister(
	corev1.RegisterUserServiceHandlerFromEndpoint,
	bizv1.RegisterBillServiceHandlerFromEndpoint,
)
```

不要使用生成代码中的 `Register*HandlerServer`，该方式会直接调用业务实现并绕过 gRPC interceptor。

## 额外 HTTP 接口

```go
err = server.HandlePath(
	http.MethodPost,
	"/upload",
	func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
		// 校验并处理上传内容。
		w.WriteHeader(http.StatusNoContent)
	},
)
```

`HandlePath` 只能在 `Start` 前调用。额外 HTTP 接口会经过 HTTP Request ID、访问日志和 Recovery，但没有 protobuf 消息，因此不会应用 Protovalidate。

## 启动与停止

```go
if err := lifex.Init(); err != nil {
	lifex.Shutdown(err)
}
if err := lifex.Wait(); err != nil {
	return err
}
```

`New` 会将 Server 的 `Start` 和 `Stop` 注册到全局 `lifex`。`lifex.Init` 完成监听和 Gateway 注册后异步启动服务；`lifex.Wait` 阻塞等待 SIGINT、SIGTERM 或 `lifex.Shutdown`。服务异常退出时，错误会通过 `lifex.Shutdown` 传递给 `Wait`。

也可以由业务逻辑主动触发退出：

```go
lifex.Shutdown(reason)
```

`lifex.Wait` 会逆序执行全部解构函数。Server 的 `Stop` 会并行排空 HTTP 与 gRPC 请求，并使用 `WithGracefulTimeout` 配置的超时完成优雅停止，默认五秒；Server 只能启动一次，`Stop` 可以重复调用。

## TLS

```go
server, err := standard.New(
	standard.WithAddress(":8443"),
	standard.WithCertificate("server.crt", "server.key"),
	standard.WithGatewayServerName("api.example.com"),
	// 其他服务注册 Options。
)
```

证书和私钥必须同时指定。Gateway 默认使用证书文件作为回连 gRPC 的信任根，并使用真实 endpoint 主机名进行校验；监听通配地址或证书名称不同时，应通过 `WithGatewayServerName` 指定证书名称。

复杂 TLS、动态证书和 mTLS 可以使用 `WithTLSConfig`。如果 Gateway 需要独立的客户端 TLS 策略，可通过 `WithGatewayDialOptions` 覆盖默认传输凭据；不得使用跳过证书校验作为生产默认配置。

## Options

| Option                   | 用途                                              |
|--------------------------|---------------------------------------------------|
| `WithAddress`            | 设置监听地址，默认 `:8080`                        |
| `WithListener`           | 注入 Listener，优先于 Address                     |
| `WithGracefulTimeout`    | 设置 lifex 解构阶段的停止超时，默认五秒           |
| `WithCertificate`        | 设置 TLS 证书和私钥文件                           |
| `WithTLSConfig`          | 注入自定义 TLS 配置                               |
| `WithGatewayEndpoint`    | 为非 TCP Listener 或特殊网络覆盖 Gateway endpoint |
| `WithGatewayServerName`  | 设置 Gateway TLS 回连校验名称                     |
| `WithGatewayDialOptions` | 追加或覆盖 Gateway gRPC 客户端配置                |
| `WithGatewayOptions`     | 注入 grpc-gateway Mux Options                     |
| `WithGRPCServerOptions`  | 注入 gRPC Server Options                          |
| `WithHTTPServerOptions`  | 调整 HTTP Server 超时等参数，Handler 不允许替换   |
| `WithUnaryInterceptors`  | 在标准校验和 Recovery 之间插入 unary interceptor  |
| `WithStreamInterceptors` | 在标准校验和 Recovery 之间插入 stream interceptor |
| `WithGRPCRegister`       | 注册真实 gRPC 服务                                |
| `WithGatewayRegister`    | 注册 `google.api.http` 生成的 Gateway endpoint    |
| `WithLogger`             | 替换默认的 `core/logx` gRPC 日志适配器            |

## 默认 Middleware

gRPC interceptor 顺序为：

```text
Request ID -> Logging -> Protovalidate -> 自定义 Interceptor -> Recovery
```

- Request ID 使用 `X-Request-ID` 和 gRPC `x-request-id` metadata；缺失时生成 UUID v7。
- 业务代码可以通过 `standard.RequestID(ctx)` 读取请求标识，`core/logx.Ctx(ctx)` 也会自动携带该字段。
- Logging 记录协议、方法、状态、耗时和请求 ID，不记录 payload、认证头或完整 metadata。
- 方法设置 `(options.method).skip_log = true` 时跳过对应的 gRPC 访问日志。
- Protovalidate 执行 Protobuf 中的 `buf.validate` 规则，失败时返回 `InvalidArgument`。
- Recovery 将 gRPC panic 转换为 `Internal`，并保护额外 HTTP Handler 不导致进程退出。

## 开发约定

修改代码前先阅读 `AGENTS.md`、`PROJECT_MAP.md` 和本文件。未经明确授权，不运行测试、示例服务或任何真实外部调用。

`make run` 会先构建并直接启动示例二进制；`make build` 会先创建 `bin` 目录，再生成 `bin/server`。
