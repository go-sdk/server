package main

import "embed"

// errorI18nFS 保存示例服务和测试服务共用的错误文案翻译文件。
// 默认英文文案由错误码的 message 选项提供，embed 文件只维护其他语言。
//
//go:embed locales/*.toml
var errorI18nFS embed.FS
