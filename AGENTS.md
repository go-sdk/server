# 仓库协作规范

## 文档优先

每次修改代码前，必须先阅读以下文档：

1. `AGENTS.md`
2. `PROJECT_MAP.md`
3. `README.md`

如果文档描述与实际代码不一致，应在同一次修改中更新相关文档，确保文档反映最终实现。

## 修改原则

- 这是个人 Go 服务基础类库，优先保持 API 简洁、生命周期明确和向后兼容。
- 修改前检查目录结构、调用关系和 Git 工作区状态，不覆盖或混入无关改动。
- 遵循现有包划分和代码风格，优先局部修改，不引入缺少明确收益的依赖或抽象。
- 代码注释、维护文档和面向使用者的说明统一使用简体中文。
- 注释只描述最终设计意图、业务含义和关键约束，不记录修改历史，不复述代码。
- 日志和错误不得包含证书私钥、认证信息、JWT 原文或完整 metadata；Payload Logging 必须按字段选项脱敏。

## 设计约束

- 对外业务能力以 gRPC 服务为准，HTTP API 由 Protobuf 中的 `google.api.http` 注解生成。
- grpc-gateway 必须通过 gRPC ClientConn 调用同一进程的真实 gRPC Server，不使用绕过 interceptor 的直接实现注册方式。
- HTTP 和 gRPC 通过 gmux 共用同一个真实 Listener；HTTP 使用 `runtime.ServeMux`，gRPC 使用 gmux 返回的虚拟 Listener。
- 只有上传、下载或 Webhook 等无法合理映射为 RPC 的接口才使用 `Server.HandlePath` 注册。
- 不暴露底层 `runtime.ServeMux`、`http.Server` 或 `grpc.Server`，服务和 Gateway 注册通过 Options 注入。
- Server 默认 interceptor 顺序为 Request Context、Logging、Payload Logging、JWT Auth、Protovalidate、自定义 interceptor、Recovery。
- Client 默认使用明文连接，并通过 Option 启用 TLS；默认 interceptor 负责请求上下文透传、Logging 和 Payload Logging。
- `standard.NewContext` 和 `standard.FromContext` 统一管理 Request ID、JWT Claims、调用深度及安全的请求信息；认证原文仅供内部透传。
- JWT 鉴权仅在注入 HS256 密钥后启用；Health Service 固定跳过鉴权和 Payload Logging，Reflection 默认关闭。
- Listener 非空时优先于 Address。Server 只能启动一次，`Stop` 必须支持重复调用并使用配置的超时完成优雅停止。
- Server 通过 `core/lifex` 注册启动和停止函数，由 `lifex.Init`、`lifex.Wait` 和 `lifex.Shutdown` 统一管理进程生命周期。
- 错误创建、包装和判断统一使用 `core/errx`；错误文本和日志消息使用小写字母开头。
- TLS 证书和私钥通过文件路径或 PEM Option 注入；不得输出私钥内容到代码和日志。

## 验证边界

- 修改后使用项目已有的格式化工具，并检查 `git diff`、`git diff --check` 和工作区状态。
- 优先依次运行 `make lint` 和纯编译检查；不得把静态检查或编译成功描述为运行时验证。
- 未经用户明确授权，不运行单元、集成、端到端或冒烟测试，不启动示例服务，不访问真实接口、数据库、消息队列或云资源。
- 不执行部署、发布、上传或 `git push`；本地提交需要用户明确要求。

## 提交规范

- 提交信息遵循 Conventional Commits。
- 标题使用简洁英文；需要补充背景、影响或风险时写入提交正文，正文每行不超过 80 个字符。
- 只提交当前任务明确包含并已复核的文件。
