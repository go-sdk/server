# server

`server` 是个人使用的 Go 服务基础类库，模块路径为 `github.com/go-sdk/server`。项目使用 gmux 在同一个端口提供原生 gRPC 和 grpc-gateway HTTP API，通过 `core/lifex` 管理进程生命周期，并统一请求上下文、访问日志、Payload Logging、JWT 鉴权、权限与审计函数调用、Protovalidate、Health、Recovery、TLS 和优雅停止行为。

## 环境要求

- Go 1.27 或更高版本

## 安装

```bash
go get github.com/go-sdk/server
```

## 公共 Protobuf

`buf.build/go-sdk/server` 公共模块同时包含 `server.common` 通用类型和 `server.options` 自定义选项。业务协议可以直接引用：

```protobuf
import "common/common.proto";
import "options/options.proto";

message ListUserReq {
  server.common.Paging paging = 1;
}
```

`server.common` 提供单个和批量标识、资源元数据、动态属性、分页和时间范围，并通过 public import 传递常用 Google Protobuf 类型。`TimeRange` 在首尾时间都存在时要求开始时间不得晚于结束时间。

Go 代码使用仓库内生成包，以便同时获得手写辅助方法：

```go
import "github.com/go-sdk/server/common"

paging := common.NewPaging(2, 20)
offset, limit := paging.GetOffsetLimit()
responsePaging := paging.WithTotal(100)
```

`NewPaging` 使用与分页计算方法相同的默认值构造 `Paging`：页码非正数时按第 `1` 页处理，每页数量非正数时使用 `common.DefaultLimit`。`GetOffsetLimit`、`GetOffset` 和 `GetLimit` 对手动构造的 `Paging` 也使用相同规则；`WithTotal` 返回新的分页对象，不修改原请求。BSR 自动生成的 Go SDK 不包含 `common/common.go` 中的手写方法；需要这些方法的 Go 项目应依赖 `github.com/go-sdk/server/common`。

`master` CI 会持续将根模块中 `includes` 声明的公共 Proto 发布到 `buf.build/go-sdk/server` 的 `master` label，tag CI 则发布对应版本 label；未命名的测试模块不会发布。后续增加公共 Proto 目录时，需要同步将该目录加入 `buf.yaml` 的 `includes`。

## 服务模型

业务接口以 Protobuf gRPC 服务为唯一来源，通过 `google.api.http` 注解生成 HTTP API。Gateway 使用生成的 `Register*HandlerFromEndpoint` 回连同端口的真实 gRPC Server，因此 HTTP 和原生 gRPC 请求经过相同的 gRPC interceptor。

上传、下载或 Webhook 等不适合建模为 RPC 的接口，可以通过 `Server.HandlePath` 直接注册到内部 Gateway Mux。底层 `runtime.ServeMux`、`http.Server` 和 `grpc.Server` 不对外暴露。

## HTTP 返回结构

Gateway 成功响应统一将 Protobuf 消息放在 `data` 下。成功码为零并默认隐藏：

```json
{
  "data": {
    "id": "1"
  }
}
```

失败响应保留 gRPC `code`、`message` 和结构化 `details`，并增加业务错误码 `domain` 和本地化文案 `reason`。`details` 固定为最后一个字段，未包含结构化详情时输出空数组：

```json
{
  "code": 5,
  "message": "resource not found",
  "domain": "1000002",
  "reason": "文件 report.pdf 不存在",
  "details": []
}
```

业务错误码通过枚举定义。枚举数字值写入 `domain`，`message` 是英文默认文案；`http_status` 只覆盖 `HandlePath` 的 HTTP 状态码：

```protobuf
enum ErrorCode {
  ERROR_CODE_UNSPECIFIED = 0;
  ERROR_CODE_FILE_NOT_FOUND = 1000002 [
    (server.options.enum_value).http_status = 404,
    (server.options.enum_value).message = "file {{.Name}} not found"
  ];
}
```

业务代码以不可变的 `standard.Err*` 为基座，通过 `WithErrorCode` 绑定错误码，`WithData` 提供文案模板变量：

```go
return nil, standard.ErrNotFound.
	WithErrorCode(common.ErrorCode_ERROR_CODE_FILE_NOT_FOUND).
	WithData(map[string]any{"Name": "report.pdf"})
```

默认语言是英文。Server 根据 HTTP `Accept-Language` 或 gRPC `accept-language` metadata 选择翻译；目标语言不存在时回退到枚举 option 中的英文 `message`，英文文案缺失或模板渲染失败（包括缺少 `WithData` 提供的模板变量）时回退到枚举名称。`reason` 是最终渲染结果，`domain` 是枚举数字值的十进制字符串。两者通过 `google.rpc.ErrorInfo` 在 gRPC 中传递，Gateway 会提升到失败响应顶层，不在 `details` 中重复输出。

推荐由业务服务使用 TOML 文件维护翻译，并通过 `embed.FS` 注入。Server 自动以英文创建 Bundle、注册 TOML 解析器并递归加载全部 `.toml` 文件；翻译消息 ID 使用 `domain` 的数字字符串。例如 `locales/zh-CN.toml`：

```toml
["1000001"]
other = "无效的文件名"

["1000002"]
other = "文件 {{.Name}} 不存在"
```

业务入口只需嵌入语言目录：

```go
//go:embed locales/*.toml
var localeFS embed.FS

server, err := standard.New(
	standard.WithI18nFS(localeFS),
	// ...
)
```

语言标签从文件名读取，支持 `zh-CN.toml` 或 `active.zh-CN.toml`。没有 TOML 文件、文件名语言标签无效或内容解析失败时，`standard.New` 直接返回错误。英文默认文案继续来自错误码的 `message` option，不要求提供 `en.toml`。需要自行配置其他格式或复数规则时，可以继续使用 `WithI18nBundle` 注入完整 Bundle。

优先使用 `ErrInternal`、`ErrInvalidParam`、`ErrUnauthenticated`、`ErrNotFound`、`ErrPermissionDenied`、`ErrAlreadyExists`、`ErrResourceExhausted`、`ErrFailedPrecondition`、`ErrAborted` 和 `ErrUnavailable` 等不可变的内置错误模板；只有缺少对应模板时才使用 `standard.NewError(code, message)`。`WithMessage`、`WithErrorCode`、`WithData`、`WithDomainReason`、`WithDetails` 和 `WithHTTPStatus` 都返回副本，可以安全地复用错误模板。`WithDomainReason` 保留给不使用错误码枚举的业务。`WithHTTPStatus` 不改变原生 gRPC Code。

数据库、缓存或第三方 SDK 的错误可以通过 `ErrorConverter` 统一转换，而不让 Server 直接依赖具体驱动：

```go
standard.WithErrorConverters(
	standard.ErrorConvertFunc(func(err error) (standard.RespError, bool) {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return standard.ErrNotFound.WithMessage("record not found").
				WithDomainReason("RECORD_NOT_FOUND", "record.not_found"), true
		case errors.Is(err, gorm.ErrRecordNotFound):
			return standard.ErrNotFound.WithMessage("record not found").
				WithDomainReason("RECORD_NOT_FOUND", "record.not_found"), true
		default:
			return standard.RespError{}, false
		}
	}),
)
```

转换器按注入顺序执行，第一个返回 `true` 的结果生效；未匹配错误保持不变。转换同时覆盖 unary 和 stream RPC，应用可以使用 `errors.Is` 或 `errors.As` 识别被包装的依赖错误。

## 创建服务

```go
userService := NewUserService()

server, err := standard.New(
	standard.WithName("user-service"),
	standard.WithAddress(":8080"),
	standard.WithJWTSecret([]byte(os.Getenv("JWT_SECRET"))),
	standard.WithReflection(),
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
	func(c *standard.Context) error {
		if _, _, err := c.ReadFormFile("file", 32<<20); err != nil {
			return err
		}
		return c.NoContent(http.StatusNoContent)
	},
)
```

`HandlePath` 接收 `standard.HandlerFunc`，其签名为 `func(c *standard.Context) error`，并且只能在 `Start` 前调用。Handler 返回 `nil` 表示响应已经正常完成；返回 `RespError` 时，Server 会按照与 Gateway 相同的 `code`、`message`、`domain`、`reason` 和 `details` 结构写入响应，并根据 gRPC Code 设置 HTTP 状态码。已注册的 Error Converter 同样作用于额外 HTTP Handler；未匹配的普通错误不会向调用方暴露原始信息，而是返回 `ErrInternal`。如果响应体已经开始写入，后续返回的错误只会记录日志，不会覆盖已提交的响应。

`Context` 通过 `Request` 和 `Response` 暴露原始 `*http.Request` 与 `http.ResponseWriter`，同时提供 `Param`、`Query`、`Queries`、`Header`、`FormValue`、`FormFile`、`ReadFormFile`、`SetHeader`、`JSON`、`Text`、`Blob` 和 `NoContent` 等常用辅助方法。`ReadFormFile(name, maxFileBytes)` 分别限制文件内容和包含固定表单开销的请求体，使用受控的内存阈值解析 multipart 表单，并负责关闭文件及清理临时文件；请求或文件超限返回 HTTP 413 和 `ErrResourceExhausted`，表单或字段无效返回 `ErrInvalidParam`。`NewHTTPContext` 可用于不启动 Server 的 Handler 单元测试。

额外 HTTP 接口会经过 HTTP 链路标识（TraceID/SpanID）、访问日志和 Recovery，但没有 protobuf 消息，因此不会应用 Protovalidate。

配置 JWT 后，额外 HTTP 接口同样要求 `Authorization: Bearer <token>`，并可直接通过 `c.JWT()` 读取已验证的 Claims 和请求参数。鉴权失败和 Handler panic 也使用统一错误结构。额外接口没有 Protobuf MethodOptions，不能使用 `skip_auth`。

## 示例接口测试

`tests/docs/api.http` 提供可由 JetBrains HTTP Client 执行的示例接口测试，覆盖 Gateway 的创建、参数校验和分页响应，以及额外 HTTP 路由的上传、下载、鉴权和错误文案本地化。运行前启动 `tests/server`，并选择 `tests/docs/http-client.env.json` 中的 `local` 环境。

`local.token` 仅用于本仓库示例服务，由与 `tests/server/main.go` 相同的测试密钥 `12345678` 签发。修改示例服务的 JWT 密钥时必须同步更新该 Token。上传和下载共享示例服务进程内的文件存储，因此下载成功用例需要先执行“上传文件”；重启服务后也需要重新上传。Protobuf 的零值字段默认不输出，分页用例只断言请求中实际指定的页码和每页数量。

## 启动与停止

```go
if err := lifex.Init(); err != nil {
	lifex.Shutdown(err)
}
if err := lifex.Wait(); err != nil {
	return err
}
```

`New` 会将 Server 的 `Start` 和 `Stop` 注册到全局 `lifex`。`lifex.Init` 完成监听和 Gateway 注册后异步启动服务；`lifex.Wait` 阻塞等待 SIGINT、SIGTERM 或 `lifex.Shutdown`。服务异常退出时，错误会通过 `lifex.Shutdown` 传递给 `Wait`。配置 `WithName` 后，启动、停止日志会增加 `server` 字段，HTTP 或 gRPC Serve 异常也会包含该实例名称；该名称不会写入请求 metadata，也不会修改 `core/logx` 的进程级全局字段。

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

也可以直接注入内存中的 PEM 内容：

```go
server, err := standard.New(
	standard.WithAddress(":8443"),
	standard.WithCertificatePEM(certPEM, keyPEM),
	standard.WithGatewayServerName("api.example.com"),
	// 其他服务注册 Options。
)
```

证书和私钥必须同时指定。Gateway 默认使用注入的证书文件或证书 PEM 作为回连 gRPC 的信任根，并使用真实 endpoint 主机名进行校验；监听通配地址或证书名称不同时，应通过 `WithGatewayServerName` 指定证书名称。

复杂 TLS、动态证书和 mTLS 可以使用 `WithTLSConfig`。如果 Gateway 需要独立的客户端 TLS 策略，可通过 `WithGatewayDialOptions` 覆盖默认传输凭据；不得使用跳过证书校验作为生产默认配置。

## Options

| Option                   | 用途                                                 |
|--------------------------|------------------------------------------------------|
| `WithName`               | 设置生命周期日志和 Serve 异常中的 Server 实例标识    |
| `WithAddress`            | 设置监听地址，默认 `:8080`                           |
| `WithListener`           | 注入 Listener，优先于 Address                        |
| `WithGracefulTimeout`    | 设置 lifex 解构阶段的停止超时，默认五秒              |
| `WithCertificate`        | 设置 TLS 证书和私钥文件                              |
| `WithCertificatePEM`     | 设置内存中的 PEM 证书和私钥                          |
| `WithTLSConfig`          | 注入自定义 TLS 配置                                  |
| `WithGatewayEndpoint`    | 为非 TCP Listener 或特殊网络覆盖 Gateway endpoint    |
| `WithGatewayServerName`  | 设置 Gateway TLS 回连校验名称                        |
| `WithGatewayDialOptions` | 追加或覆盖 Gateway gRPC 客户端配置                   |
| `WithGatewayOptions`     | 注入 grpc-gateway Mux Options                        |
| `WithGRPCServerOptions`  | 注入 gRPC Server Options                             |
| `WithHTTPServerOptions`  | 调整 HTTP Server 超时等参数，Handler 不允许替换      |
| `WithUnaryInterceptors`  | 在标准校验和 Recovery 之间插入 unary interceptor     |
| `WithStreamInterceptors` | 在标准校验和 Recovery 之间插入 stream interceptor    |
| `WithErrorConverters`    | 将数据库等应用依赖错误转换为 `RespError`             |
| `WithI18nFS`             | 从 embed.FS 递归加载 TOML 错误文案                   |
| `WithI18nBundle`         | 注入业务错误文案的 go-i18n 翻译目录                  |
| `WithGRPCRegister`       | 注册真实 gRPC 服务                                   |
| `WithGatewayRegister`    | 注册 `google.api.http` 生成的 Gateway endpoint       |
| `WithLogger`             | 替换默认的 `core/logx` gRPC 日志适配器               |
| `WithJWTSecret`          | 注入 HS256 密钥并启用 JWT 鉴权                       |
| `WithJWTAuth`            | 注入自定义 `jwtx.Parser`，支持非对称算法和外部密钥源 |
| `WithPermissionFunc`     | 注入业务权限校验方法                                 |
| `WithAuditFunc`          | 注入业务审计记录方法                                 |
| `WithReflection`         | 启用标准 gRPC Reflection Service                     |

## 默认 Middleware

gRPC interceptor 顺序为：

```text
Request Context -> Logging -> Payload Logging -> JWT Auth -> Permission -> Protovalidate -> Audit -> 自定义 Interceptor -> Error Converter -> Recovery
```

- Request Context 对外使用 `X-Request-Id` 请求头和 gRPC `x-request-id` metadata 接收与回写链路标识；内部统一命名为 TraceID（`standard.TraceIDKey`，日志字段 `trace-id`），缺失时生成 UUID v7，并在同一进程内透传。每次请求进入服务时生成新的 SpanID（`standard.SpanIDKey`，日志字段 `span-id`），标识本服务内单次请求到响应的处理，不向下游透传，也不出现在任何响应中。TraceID、SpanID、调用深度、客户端 IP、content-type、user-agent 和原始 `Accept-Language` 会写入 `standard.Context`，语言偏好还会透传给下游 gRPC 服务。
- `core/logx.Ctx(ctx)` 自动携带 `trace-id`、`span-id` 和 depth；业务代码通过 `standard.FromContext(ctx)` 读取请求参数和 JWT Claims。
- Logging 记录协议、方法、状态和耗时；Payload Logging 分别输出 `grpc request` 和 `grpc response`，不记录认证头、JWT 原文或完整 metadata。
- Payload Logging 的 `content_length` 是 `proto.Size` 得到的逻辑消息长度，不代表压缩和 HTTP/2 帧编码后的网络字节数。
- 方法设置 `(server.options.method).skip_log = true` 时仍记录请求与响应元数据，但 payload 使用 `***`；字段设置 `(server.options.field).sensitive = true` 时递归脱敏。
- 配置 `WithJWTSecret` 后使用 HS256 验证 Bearer Token，`WithJWTAuth` 可注入非对称算法和外部密钥源的 `jwtx.Parser`；方法设置 `(server.options.method).skip_auth = true` 时跳过鉴权。
- 配置 `WithPermissionFunc` 后，SDK 将方法名和 `(server.options.method).permissions` 交给业务方法校验；未声明 MethodOptions 的业务 RPC 按配置错误拒绝。
- Protovalidate 执行 Protobuf 中的 `buf.validate` 规则，失败时返回 `InvalidArgument`。
- 配置 `WithAuditFunc` 后，非空 `(server.options.method).audit_kind` 会在 RPC 完成时调用业务审计方法。Unary RPC 携带已按 `sensitive` 和 `skip_log` 脱敏的请求与响应；流式 RPC 只携带方法、类型、结果和耗时。回调错误仅记录，不改写已完成的业务结果。
- Error Converter 按注册顺序将应用依赖错误转换为统一 `RespError`；额外 HTTP Handler 的未匹配错误统一隐藏为 `ErrInternal`。
- Recovery 将 gRPC panic 转换为 `Internal`，额外 HTTP Handler 的 panic 转换为统一 `ErrInternal` 响应。

## 请求上下文

```go
requestContext := standard.FromContext(ctx)
traceID := requestContext.TraceID()
spanID := requestContext.SpanID()
clientIP := requestContext.ClientIP()
depth := requestContext.Depth()
claims := requestContext.JWT()
acceptLanguage := requestContext.AcceptLanguage()
```

`TraceID` 是跨服务透传的链路标识，对外线上协议固定为 `X-Request-Id` 请求头和 gRPC `x-request-id` metadata；`SpanID` 由每个服务在请求入口生成，只标识本服务内这一次请求到响应的处理，仅随日志输出，不随出站调用传递，也不写入 HTTP 响应头或 gRPC 响应 metadata。HTTP 响应只会写一个 `X-Request-Id`；Gateway 不透传任何 gRPC 响应头，因此 HTTP 响应中不会出现 `Grpc-Metadata-` 前缀的头或重复的 `X-Request-Id`。

`JWT()` 返回已验证 Claims 的副本，不包含原始 Bearer Token；`JWTClaims(target)` 将 Claims 解码到自定义结构，用于业务侧读取强类型声明。未配置鉴权 Option 时不启用鉴权。标准 gRPC Health Service 始终启用，并固定跳过鉴权和 Payload Logging；Reflection 默认关闭，仅通过 `WithReflection()` 启用，启用后仍遵循 JWT 鉴权。

JWT 的签发和解析统一由 `github.com/go-sdk/server/jwtx` 提供：`jwtx.Signer` 以指定算法和密钥签发 token，`jwtx.Parser` 按算法白名单和密钥函数解析，`jwtx.Codec` 内嵌两者同时提供签发和解析。`jwtx.HS256(secret)` 返回对称密钥 Codec，`jwtx.Ed25519(privateKeyPEM)` 从 PEM 编码的 PKCS#8 私钥构造私钥签发、对应公钥解析的 Codec；仅需解析时直接使用 `codec.Parser`。`jwtx.Claims` 提供 jti/sub/iat/exp 四个标准声明字段（时间为 `jwtx.Time`，payload 内使用秒级时间戳），其余声明与标准声明同级扁平存放在 `Extra` 并可通过 `Decode` 解码到目标结构；`jwtx.MapClaims` 与服务端中间件写入上下文的声明类型一致。

需要为主动发起的调用补充请求参数时，使用 `standard.NewContext`：

```go
ctx = standard.NewContext(ctx,
	standard.TraceIDKey, traceID,
	standard.DepthKey, depth,
)
```

## gRPC Client

```go
conn, err := standard.NewClient("dns:///user-service:8080")
if err != nil {
	return err
}
client := corev1.NewUserServiceClient(conn)
```

Client 默认使用明文连接，并由 `lifex` 在解构阶段关闭。默认 interceptor 负责 Logging、Payload Logging，以及从 `standard.Context` 透传 `x-request-id` metadata、已验证 JWT 对应的 Bearer Token，并将 `x-depth` 加一。SpanID 不透传，由下游服务在请求入口自行生成。认证原文不会通过公共 Context API 暴露或写入日志。

通过根证书文件、根证书 PEM 或完整 TLS 配置启用 TLS：

```go
conn, err := standard.NewClient(
	"dns:///user-service:8443",
	standard.WithClientRootCertificatePEM(rootCertPEM),
	standard.WithClientServerName("user-service.example.com"),
)
```

可用的 Client Options 包括 `WithClientTLSConfig`、`WithClientRootCertificate`、`WithClientRootCertificatePEM`、`WithClientServerName`、`WithClientDialOptions`、`WithClientUnaryInterceptors`、`WithClientStreamInterceptors` 和 `WithClientLogger`。

## 单元测试

`standard/testserver` 使用 `bufconn` 启动真实标准 Server，并提供经过默认 Client interceptor 的连接。测试会自动释放 ClientConn、Server 和 Listener：

```go
server := testserver.New(t,
	standard.WithGRPCRegister(func(registrar grpc.ServiceRegistrar) {
		corev1.RegisterUserServiceServer(registrar, &userService{})
	}),
)
client := corev1.NewUserServiceClient(server.Conn())

response, err := client.Health(context.Background(), &common.Empty{})
```

`testserver` 固定注入 `bufconn` Listener，适用于原生 gRPC 和 interceptor 单元测试。需要真实 TCP endpoint 的 grpc-gateway HTTP 路由应使用独立的集成测试。

额外 HTTP Handler 可以使用 `NewHTTP` 在本地回环端口走完整的标准 HTTP 链路：

```go
server := testserver.NewHTTP(t, http.MethodPost, "/files", uploadHandler,
	standard.WithErrorConverters(databaseErrorConverter),
)
request, err := http.NewRequest(http.MethodPost, server.URL("/files"), body)
if err != nil {
	t.Fatal(err)
}
response, err := server.Client().Do(request)
if err != nil {
	t.Fatal(err)
}
defer response.Body.Close()
```

`NewHTTP` 只负责注册一个 `HandlePath` Handler、启动 Server 并管理 HTTP Client 和资源清理，不初始化应用配置、数据库或迁移。普通 grpc-gateway 生成路由仍应通过对应 Service 的测试覆盖，涉及真实外部依赖时使用独立的集成测试。

## 开发约定

修改代码前先阅读 `AGENTS.md`、`PROJECT_MAP.md` 和本文件。未经明确授权，不运行测试、示例服务或任何真实外部调用。

`make run` 会先构建并直接启动示例二进制；`make build` 会先创建 `bin` 目录，再生成 `bin/server`。
