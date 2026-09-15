# 项目地图

## 项目定位

`github.com/go-sdk/server` 是个人使用的 Go 服务基础类库，负责在同一个端口上提供 gRPC 与 grpc-gateway HTTP API，并统一服务初始化、标准 middleware、TLS 和优雅停止行为。

## 目录结构

```text
server/
├── common/                         跨服务公共 Protobuf 类型和 Go 辅助方法（proto 包为 server.common）
│   ├── common.proto                标识、元数据、动态属性、分页和时间范围定义
│   ├── common.go                   公共类型构造方法和分页计算辅助方法
│   ├── common.pb.go                生成的 Go 消息定义
│   └── common.pb.json.go           生成的 JSON 编解码方法
├── options/                        公共 Protobuf 方法、消息和字段选项（proto 包为 server.options）
│   ├── options.proto               认证、日志和敏感字段描述
│   ├── options.pb.go               生成的 Go 扩展定义
│   └── options.pb.json.go          生成的 JSON 编解码方法
├── standard/                       标准单端口 gRPC/Gateway Server
│   ├── logger.go                   core/logx 的 gRPC logging 适配
│   ├── client.go                   gRPC ClientConn 创建和 lifex 生命周期注册
│   ├── client_options.go           Client 明文、TLS 和 middleware Options
│   ├── client_middleware.go        Client 上下文透传和 Payload Logging
│   ├── context.go                  TraceID、SpanID、JWT Claims 和调用深度
│   ├── http_context.go             额外 HTTP Handler、请求参数和响应辅助方法
│   ├── http_handler.go             额外 HTTP 路由适配、鉴权和统一错误响应
│   ├── error.go                    业务响应错误及内置错误模板
│   ├── error_converter.go          应用依赖错误转换接口和 interceptor
│   ├── i18n.go                     错误文案本地化及 embed TOML 加载
│   ├── gateway.go                  Gateway 注册和回连 endpoint 解析
│   ├── lifecycle.go                lifex 启动、异常退出和优雅停止
│   ├── access_log.go               HTTP 访问日志
│   ├── auth.go                     JWT 鉴权
│   ├── method_options.go           方法选项解析
│   ├── middleware.go               公共 middleware 类型和 Protovalidate
│   ├── options.go                  Server 初始化 Options 和注册函数类型
│   ├── payload_logging.go          gRPC 请求和响应 Payload Logging
│   ├── recovery.go                 HTTP 与 gRPC Recovery
│   ├── request_context.go          TraceID、SpanID 和请求上下文
│   ├── response.go                 Gateway 成功和失败响应结构
│   ├── server.go                   Server 构造、gmux 和额外路由注册
│   ├── tls.go                      TLS 判断和 Gateway 客户端凭据
│   └── testserver/                 标准 gRPC 与额外 HTTP 接口测试服务器
│       ├── http.go                 回环 HTTP Server、Client、URL 和自动清理
│       └── server.go               bufconn Server、ClientConn 和自动清理
├── tests/
│   ├── docs/                       JetBrains HTTP Client 接口测试和本地环境配置
│   ├── pb/                         由 Buf 生成的测试及示例代码
│   ├── openapi/                    由 Buf 生成的 Swagger 2.0 文档（openapi.swagger.yaml）
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

## 公共 Protobuf 模块

- `buf.build/go-sdk/server` 以仓库根目录为模块路径，仅包含 `common` 和 `options` 两个 Proto 目录。
- `common/common.proto` 定义 `server.common` 公共类型，并通过 public import 向依赖方传递常用 Google Protobuf 类型。
- `options/options.proto` 定义 `server.options` 方法、消息和字段扩展。
- Buf 生成代码分别落在 `common` 和 `options` Go 包；`common/common.go` 为手写文件，不得由生成或清理流程覆盖。
- 业务 Proto 通过 `common/common.proto` 和 `options/options.proto` 引用公共定义；Go 代码从 `github.com/go-sdk/server/common` 使用生成类型和手写辅助方法。
- `master` CI 持续发布 `master` label，tag CI 发布对应版本 label；两者都从 workspace 根目录推送全部命名模块，并排除 `tests/proto` 这类未命名的本地测试模块。后续新增对外 Proto 目录时，将其加入根模块的 `includes` 即可沿用同一发布流程。

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

Gateway 将成功的 Protobuf 消息放入 `data`，成功码为零且默认不输出。失败响应依次输出 `code`、`message`、`domain`、`reason` 和 `details`；错误码枚举数字值写入 `domain`，本地化后的最终文案写入 `reason`，承载它们的 `google.rpc.ErrorInfo` 不在 HTTP `details` 中重复输出。原生 gRPC 响应和 Status 结构保持不变。

## `standard` 包

### 初始化

- `New` 应用 Options，创建 Protovalidate、标准 interceptor、`grpc.Server`、`runtime.ServeMux` 和 `http.Server`，然后向 `core/lifex` 注册启动和停止函数。
- `WithName` 设置可选的 Server 实例标识，用于生命周期日志和 Serve 异常，不修改进程级全局日志字段。
- `WithGRPCRegister` 注册 gRPC 服务。
- `WithGatewayRegister` 收集生成的 Gateway endpoint 注册函数。
- `WithI18nFS` 以英文为默认语言，从传入的 `fs.FS` 递归加载 TOML 翻译文件；`WithI18nBundle` 保留为自定义 Bundle 的高级入口。
- gmux 在 HTTP Handler 完成后配置，并返回供 `grpc.Server.Serve` 使用的虚拟 Listener。

### 路由

- 主要 HTTP API 来自 Protobuf 的 `google.api.http` 注解。
- `HandlePath` 只用于无法合理建模为 gRPC 的额外 HTTP 接口，并且只能在 `Start` 前调用；Handler 使用 `func(c *standard.Context) error` 签名。
- 额外 HTTP Context 暴露原始 Request、Response 和路径参数，并提供 Query、Header、表单文件读取及 JSON、文本、二进制响应辅助方法。
- Handler 返回的 `RespError` 使用与 Gateway 相同的失败结构，默认按照 gRPC Code 映射 HTTP 状态码，并可通过错误码枚举的 `http_status` 或 `WithHTTPStatus` 覆盖额外 HTTP 接口的状态码；Error Converter、JWT 失败和 panic 也进入该响应链路，未匹配错误统一隐藏为 `ErrInternal`。
- Handler 写入响应后再返回错误时仅记录日志，不追加或覆盖已经提交的响应。
- 底层 Mux 和 Server 不作为公共 API 暴露。

### Middleware

- Request Context 对外以 `X-Request-Id` 请求头和 gRPC `x-request-id` metadata 接收并回写链路标识，内部统一命名为 TraceID；每次请求入口生成服务内 SpanID，仅随日志输出，不透传也不写入响应。两者与调用深度、安全的请求信息和原始 `Accept-Language` 一起在 HTTP Header、HTTP context、gRPC metadata 和业务 context 间传递，并注入 `standard.Context`；语言偏好继续透传给下游 gRPC 服务。Gateway 不透传任何 gRPC 响应头，HTTP 响应的 `X-Request-Id` 由外层 HTTP 中间件统一写入。
- Logging 记录 gRPC 调用元数据；Payload Logging 分别记录请求和响应，敏感字段以及设置 `server.options.method.skip_log` 的完整 payload 使用 `***` 替代。
- JWT Auth 使用 Option 注入的 HS256 密钥验证 Bearer Token，验证后的 Claims 写入 `standard.Context`；设置 `server.options.method.skip_auth` 的 RPC 跳过鉴权。
- Protovalidate 执行 `buf.validate` 规则，对原生 gRPC 和注解生成的 Gateway 请求生效。
- Error Converter 位于自定义 interceptor 和 Recovery 之间，按注册顺序将数据库等应用依赖错误转换为统一 `RespError`，随后根据请求语言渲染错误码文案；缺少目标语言时依次回退英文默认文案和枚举名称，未匹配错误原样返回。
- TOML 翻译文件由业务服务嵌入，语言标签从文件名解析；空文件系统、非法语言标签和解析失败都在 Server 初始化阶段返回错误。
- Recovery 位于 gRPC interceptor 链最内层；额外 HTTP Handler 由路由适配层恢复 panic 并写入统一 `ErrInternal` 响应，外层 HTTP Recovery 继续保护 Gateway。
- Health Service 默认启用且不鉴权、不记录 payload；Reflection 仅在设置 Option 后启用。

### 生命周期

- `lifex.Init` 调用 `Start`；`Start` 优先使用注入的 Listener，否则监听 Address，取得真实地址后注册 Gateway endpoint，并异步运行 HTTP 和 gRPC Serve 循环。
- 未配置证书时使用 HTTP/1.1 与 h2c gRPC，配置证书后使用 HTTPS 与 TLS gRPC。
- 配置实例名称后，Serve 循环异常通过 `lifex.Shutdown` 将带该名称的错误传递给 `lifex.Wait`。
- `lifex.Wait` 收到 SIGINT、SIGTERM 或主动退出后调用 `Stop`；`Stop` 并行排空 HTTP 与 gRPC 请求，HTTP 排空后关闭 Gateway ClientConn，超时后强制停止。
- Server 只能启动一次，`Stop` 可以重复调用。

## gRPC Client

- `NewClient` 创建默认明文的 `grpc.ClientConn`，并通过 `lifex.OnDeinit` 注册关闭函数。
- Client 可通过根证书文件、根证书 PEM 或自定义 `tls.Config` 启用 TLS。
- 默认 Client interceptor 依次执行上下文透传、Logging、Payload Logging 和自定义 interceptor。
- 出站请求透传 TraceID（`x-request-id` metadata）和内部保存的 Bearer Token，并将 depth 加一；SpanID 不透传，由下游服务在请求入口自行生成；认证原文不通过公共 Context API 暴露。

## 测试服务器

- `standard/testserver.New` 使用 `bufconn` 启动真实 `standard.Server`，并创建指向该服务且经过默认 Client interceptor 的 `grpc.ClientConn`。
- `standard/testserver.NewHTTP` 使用本地回环端口启动单个 `HandlePath` 接口，并提供 HTTP Client 和 URL，覆盖标准 HTTP 中间件、鉴权、错误转换及响应链。
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

- `make prepare` 安装 Protobuf 生成插件；`make generate` 使用 Buf lint 并重新生成 `common`、`options`、`tests/pb` 和 `tests/openapi`。
- `make lint` 会先执行 `go mod tidy`，然后运行 golangci-lint。
- `make test` 会运行竞态检测测试，必须得到用户明确授权后执行。
- `standard` 使用 `bufconn` 验证真实 gRPC Server、Health 和 Client 上下文透传，不注册需要 TCP endpoint 的 Gateway。
- `go build ./...` 只证明当前平台纯编译通过，不证明网络、TLS、gmux 分流或优雅停止的运行时行为。
