# 项目地图

## 项目定位

`github.com/go-sdk/server` 是个人使用的 Go 服务基础类库，负责在同一个端口上提供 gRPC 与 grpc-gateway HTTP API，并统一服务初始化、标准 middleware、TLS 和优雅停止行为。

## 目录结构

```text
server/
├── options/                        公共 Protobuf 方法、消息和字段选项
│   ├── options.proto               认证、日志和敏感字段描述
│   └── options.pb.go               生成的 Go 扩展定义
├── standard/                       标准单端口 gRPC/Gateway Server
│   ├── logger.go                   core/logx 的 gRPC logging 适配
│   ├── gateway.go                  Gateway 注册和回连 endpoint 解析
│   ├── lifecycle.go                lifex 启动、异常退出和优雅停止
│   ├── middleware.go               Request ID、Logging、Protovalidate 和 Recovery
│   ├── options.go                  Server 初始化 Options 和注册函数类型
│   ├── server.go                   Server 构造、gmux 和额外路由注册
│   └── tls.go                      TLS 判断和 Gateway 客户端凭据
├── tests/
│   ├── pb/                         由 Buf 生成的测试及示例代码
│   ├── proto/                      带 google.api.http 和 buf.validate 的示例协议
│   └── server/main.go              标准 Server 使用示例
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

- Request ID 在 HTTP Header、HTTP context、gRPC metadata 和业务 context 间传递，并注入 `core/logx` context logger。
- Logging 只记录请求元数据、状态和耗时，不记录 payload、认证头和完整 metadata；方法设置 `options.method.skip_log` 时跳过 gRPC 访问日志。
- Protovalidate 执行 `buf.validate` 规则，对原生 gRPC 和注解生成的 Gateway 请求生效。
- Recovery 位于 gRPC interceptor 链最内层，并在 HTTP 层保护额外 Handler。

### 生命周期

- `lifex.Init` 调用 `Start`；`Start` 优先使用注入的 Listener，否则监听 Address，取得真实地址后注册 Gateway endpoint，并异步运行 HTTP 和 gRPC Serve 循环。
- 未配置证书时使用 HTTP/1.1 与 h2c gRPC，配置证书后使用 HTTPS 与 TLS gRPC。
- Serve 循环异常时通过 `lifex.Shutdown` 将错误传递给 `lifex.Wait`。
- `lifex.Wait` 收到 SIGINT、SIGTERM 或主动退出后调用 `Stop`；`Stop` 并行排空 HTTP 与 gRPC 请求，HTTP 排空后关闭 Gateway ClientConn，超时后强制停止。
- Server 只能启动一次，`Stop` 可以重复调用。

## 主要依赖关系

```text
standard ──> gmux
         ├─> core/errx、core/lifex
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
- `go build ./...` 只证明当前平台纯编译通过，不证明网络、TLS、gmux 分流或优雅停止的运行时行为。
