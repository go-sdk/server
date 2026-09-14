package main

import (
	"os"

	"github.com/go-sdk/core/lifex"
	"github.com/go-sdk/core/logx"
	"github.com/go-sdk/core/osx"
	"google.golang.org/grpc"

	"github.com/go-sdk/server/standard"
	bizv1 "github.com/go-sdk/server/tests/pb/biz/v1"
	corev1 "github.com/go-sdk/server/tests/pb/core/v1"
)

// main 启动标准示例服务，演示 standard.Server 的完整用法：
// gRPC 与 Gateway 共用 :8080 端口，业务实现见 service.go；
// 上传和下载等无法建模为 RPC 的接口通过 HandlePath 注册，见 files.go；
// 错误文案通过注入的 i18n Bundle 本地化，见 i18n.go。
func main() {
	// 进程级全局日志字段，随所有日志输出
	logx.SetGlobalKV("app", "tests")
	logx.SetGlobalKV("version", osx.GetVersion().Version)

	// WithGRPCRegister 注册真实 gRPC 服务，Gateway 通过 ClientConn 回连同一进程，
	// HTTP 与原生 gRPC 请求经过相同的 interceptor 链；
	// WithJWTSecret 注入密钥后启用 Bearer Token 鉴权
	server, err := standard.New(
		standard.WithName("tests"),
		standard.WithAddress(":8080"),
		standard.WithReflection(),
		standard.WithJWTSecret([]byte("12345678")),
		// 从 embed.FS 加载 TOML 错误文案，错误响应按 Accept-Language 选择语言渲染 reason，
		// 未匹配的语言回退到错误码定义的英文默认文案
		standard.WithI18nFS(errorI18nFS),
		standard.WithGRPCRegister(func(registrar grpc.ServiceRegistrar) {
			corev1.RegisterUserServiceServer(registrar, &userService{})
			bizv1.RegisterBillServiceServer(registrar, &billService{})
		}),
		standard.WithGatewayRegister(
			corev1.RegisterUserServiceHandlerFromEndpoint,
			bizv1.RegisterBillServiceHandlerFromEndpoint,
		),
	)
	if err != nil {
		logx.Fatal().Err(err).Msg("failed to create server")
	}

	// 上传和下载无法合理映射为 RPC，通过 HandlePath 注册额外 HTTP 路由；
	// 必须在 lifex.Init 触发 Start 之前完成
	if err = newFileStore().registerFileHandlers(server); err != nil {
		logx.Fatal().Err(err).Msg("failed to register file handlers")
	}

	if err = lifex.Init(); err != nil {
		// 启动失败时将错误传递给 Wait 后退出
		lifex.Shutdown(err)
	}
	if err = lifex.Wait(); err != nil {
		// Wait 返回非 nil 错误表示服务异常退出（如 Serve 失败）
		logx.Error().Err(err).Msg("server stopped unexpectedly")
		os.Exit(1)
	}
}
