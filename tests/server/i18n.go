package main

import (
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// newErrorI18nBundle 构造示例服务与测试服务共用的错误文案翻译目录。
// 消息 ID 使用错误码的十进制数字字符串，与错误响应中的 domain 一致，
// 见 tests/proto/common/error.proto；默认语言为英文，英文文案由错误码的
// message 选项提供，这里只补充简体中文翻译。
func newErrorI18nBundle() *i18n.Bundle {
	bundle := i18n.NewBundle(language.English)
	bundle.MustAddMessages(language.SimplifiedChinese,
		&i18n.Message{
			ID:    "1000001",
			Other: "无效的文件名",
		},
		&i18n.Message{
			ID:    "1000002",
			Other: "文件 {{.Name}} 不存在",
		},
	)
	return bundle
}
