# 项目地图

## 项目定位

`github.com/go-sdk/server` 是个人使用的 Go 服务基础类库，负责在同一个端口上提供 gRPC 与 grpc-gateway HTTP API，并统一服务初始化、标准 middleware、TLS 和优雅停止行为。

## 目录结构

```text
server/
├── options/                        公共 Protobuf 方法、消息和字段选项（proto 包为 server.options）
│   ├── options.proto               认证、日志和敏感字段描述
│   └── options.pb.go               生成的 Go 扩展定义
├── standard/                       标准单端口 gRPC/Gateway Server
│   ├── logger.go                   core/logx 的 gRPC logging 适配
│   ├── client.go                   gRPC ClientConn 创建和 lifex 生命周期注册
│   ├── client_options.go           Client 明文、TLS 和 middleware Options
│   ├── client_middleware.go        Client 上下文透传和 Payload Logging
│   ├── context.go                  Request ID、JWT Claims 和调用深度
│   ├── gateway.go                  Gateway 注册和回连 endpoint 解析
│   ├── lifecycle.go                lifex 启动、异常退出和优雅停止
│   ├── access_log.go               HTTP 访问日志
│   ├── auth.go                     JWT 鉴权
│   ├── method_options.go           方法选项解析
│   ├── middleware.go               公共 middleware 类型和 Protovalidate
│   ├── options.go                  Server 初始化 Options 和注册函数类型
│   ├── payload_logging.go          gRPC 请求和响应 Payload Logging
│   ├── recovery.go                 HTTP 与 gRPC Recovery
│   ├── request_context.go          Request ID 和请求上下文
│   ├── server.go                   Server 构造、gmux 和额外路由注册
│   ├── tls.go                      TLS 判断和 Gateway 客户端凭据
│   └── testserver/                 基于 bufconn 的标准测试服务器
│       └── server.go               测试 Server、ClientConn 和自动清理
├── tests/
│   ├── pb/                         由 Buf 生成的测试及示例代码
│   ├── proto/                      带 google.api.http 和 buf.validate 的示例协议
│   └── server/                     标准 Server 示例及业务服务单元测试
├── AGENTS.md                       仓库协作与修改规范
├── PROJECT_MAP.md                  项目结构与调用关系
├── README.md                       使用说明与公共行为
├── Makefile                        生成、检查和测试命令
├── buf.gen.yaml                    Protobuf 生成配置
├── buf.yaml                        Protobuf 模块、依赖和 lint 配置
└── go.mod                          Go 模块和依赖定义
```

## 请求链路

```text
真实 Listener
    │
http.Server
    │
  gmux
    ├── gRPC HTTP/2 ──> gRPC 虚拟 Listener ──> grpc.Server ──> 业务实现
    │
    └── HTTP/HTTPS ──> runtime.ServeMux
                           ├── google.api.http 生成路由
                           │       └── gRPC ClientConn ──> 同端口 grpc.Server
                           └── Server.HandlePath 额外 HTTP 路由
```

Gateway 使用生成代码中的 `Register*HandlerFromEndpoint`，因此注解生成的 HTTP 请求会进入真实 gRPC Server，并经过与原生 gRPC 请求相同的 interceptor。不得使用 `Register*HandlerServer` 绕过 gRPC 调用链。

## `standard` 包

### 初始化

- `New` 应用 Options，创建 Protovalidate、标准 interceptor、`grpc.Server`、`runtime.ServeMux` 和 `http.Server`，然后向 `core/lifex` 注册启动和停止函数。
- `WithGRPCRegister` 注册 gRPC 服务。
- `WithGatewayRegister` 收集生成的 Gateway endpoint 注册函数。
- gmux 在 HTTP Handler 完成后配置，并返回供 `grpc.Server.Serve` 使用的虚拟 Listener。

### 路由

- 主要 HTTP API 来自 Protobuf 的 `google.api.http` 注解。
- `HandlePath` 只用于无法合理建模为 gRPC 的额外 HTTP 接口，并且只能在 `Start` 前调用。
- 底层 Mux 和 Server 不作为公共 API 暴露。

### Middleware

- Request Context 在 HTTP Header、HTTP context、gRPC metadata 和业务 context 间传递 Request ID、调用深度与安全的请求信息，并注入 `standard.Context` 和 `core/logx`。
- Logging 记录 gRPC 调用元数据；Payload Logging 分别记录请求和响应，敏感字段以及设置 `server.options.method.skip_log` 的完整 payload 使用 `***` 替代。
- JWT Auth 使用 Option 注入的 HS256 密钥验证 Bearer Token，验证后的 Claims 写入 `standard.Context`；设置 `server.options.method.skip_auth` 的 RPC 跳过鉴权。
- Protovalidate 执行 `buf.validate` 规则，对原生 gRPC 和注解生成的 Gateway 请求生效。
- Recovery 位于 gRPC interceptor 链最内层，并在 HTTP 层保护额外 Handler。
- Health Service 默认启用且不鉴权、不记录 payload；Reflection 仅在设置 Option 后启用。

### 生命周期

- `lifex.Init` 调用 `Start`；`Start` 优先使用注入的 Listener，否则监听 Address，取得真实地址后注册 Gateway endpoint，并异步运行 HTTP 和 gRPC Serve 循环。
- 未配置证书时使用 HTTP/1.1 与 h2c gRPC，配置证书后使用 HTTPS 与 TLS gRPC。
- Serve 循环异常时通过 `lifex.Shutdown` 将错误传递给 `lifex.Wait`。
- `lifex.Wait` 收到 SIGINT、SIGTERM 或主动退出后调用 `Stop`；`Stop` 并行排空 HTTP 与 gRPC 请求，HTTP 排空后关闭 Gateway ClientConn，超时后强制停止。
- Server 只能启动一次，`Stop` 可以重复调用。

## gRPC Client

- `NewClient` 创建默认明文的 `grpc.ClientConn`，并通过 `lifex.OnDeinit` 注册关闭函数。
- Client 可通过根证书文件、根证书 PEM 或自定义 `tls.Config` 启用 TLS。
- 默认 Client interceptor 依次执行上下文透传、Logging、Payload Logging 和自定义 interceptor。
- 出站请求透传 Request ID 和内部保存的 Bearer Token，并将 depth 加一；认证原文不通过公共 Context API 暴露。

## 测试服务器

- `standard/testserver.New` 使用 `bufconn` 启动真实 `standard.Server`，并创建指向该服务且经过默认 Client interceptor 的 `grpc.ClientConn`。
- 测试通过 `Conn` 创建生成代码中的 gRPC Client，确保业务调用经过鉴权、校验、日志和 Recovery 等真实 interceptor。
- 测试结束时自动关闭 ClientConn、Server 和 Listener；测试场景不注册依赖 TCP endpoint 的 Gateway。

## 主要依赖关系

```text
standard ──> gmux
         ├─> core/errx、core/lifex
         ├─> golang-jwt/jwt
         ├─> grpc-go
         ├─> grpc-gateway
         ├─> go-grpc-middleware
         ├─> protovalidate
         └─> core/logx、core/seq
```

## 验证边界

- `make prepare` 安装 Protobuf 生成插件；`make generate` 使用 Buf lint 并重新生成 `options` 和 `tests/pb`。
- `make lint` 会先执行 `go mod tidy`，然后运行 golangci-lint。
- `make test` 会运行竞态检测测试，必须得到用户明确授权后执行。
- `standard` 使用 `bufconn` 验证真实 gRPC Server、Health 和 Client 上下文透传，不注册需要 TCP endpoint 的 Gateway。
- `go build ./...` 只证明当前平台纯编译通过，不证明网络、TLS、gmux 分流或优雅停止的运行时行为。
