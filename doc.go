// Package server 是个人使用的 Go 服务基础类库，使用 gmux 在同一个端口提供
// 原生 gRPC 和 grpc-gateway HTTP API，并统一请求上下文、访问日志、JWT 鉴权、
// Protovalidate、Health、Recovery、TLS 和优雅停止行为。
//
// 根包本身不提供 API，主要能力位于以下子包：
//
//	standard  标准单端口 gRPC/Gateway Server、Client 和测试服务器
//	common    跨服务公共 Protobuf 类型和 Go 辅助方法
//	options   服务、方法和字段级的 Protobuf 自定义选项
//
// 安装：
//
//	go get github.com/go-sdk/server
//
// 服务创建、HTTP 返回结构、middleware 顺序和 gRPC Client 等详细说明
// 见项目 README。
package server
