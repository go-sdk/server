package standard

import (
	"context"
	"strings"

	"github.com/go-sdk/core/errx"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/nicksnyder/go-i18n/v2/i18n/template"
	"golang.org/x/text/language"
)

var (
	englishFallbackBundle = i18n.NewBundle(language.English)

	// errorTextParser 要求模板引用的变量必须由 WithData 完整提供，
	// 缺失变量按渲染失败处理，而不是把 "<no value>" 写入响应。
	errorTextParser = &template.TextParser{Option: "missingkey=error"}
)

func localizeResponseError(ctx context.Context, err error, bundle *i18n.Bundle) error {
	if err == nil {
		return nil
	}
	var responseError RespError
	if !errx.As(err, &responseError) || !responseError.hasErrorCode {
		return err
	}

	// reason 在翻译错误或模板渲染失败（含模板变量缺失）时保持为稳定的枚举名称。
	localized := responseError
	localized.reason = responseError.reason
	if strings.TrimSpace(responseError.defaultMessage) == "" {
		return localized
	}

	acceptLanguage := FromContext(ctx).AcceptLanguage()
	config := &i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    responseError.domain,
			Other: responseError.defaultMessage,
		},
		TemplateData:   responseError.data,
		TemplateParser: errorTextParser,
	}
	if bundle != nil && strings.TrimSpace(acceptLanguage) != "" {
		reason, resolvedLanguage, localizeErr := i18n.NewLocalizer(bundle, acceptLanguage).LocalizeWithTag(config)
		if localizeErr == nil && languageRequested(acceptLanguage, resolvedLanguage) && strings.TrimSpace(reason) != "" {
			localized.reason = reason
			return localized
		}
	}

	// 使用独立的英文 Bundle 渲染 proto option，避免业务 Bundle 的默认语言改变回退语义。
	reason, localizeErr := i18n.NewLocalizer(englishFallbackBundle, language.English.String()).Localize(config)
	if localizeErr == nil && strings.TrimSpace(reason) != "" {
		localized.reason = reason
	}
	return localized
}

func languageRequested(acceptLanguage string, resolved language.Tag) bool {
	requested, _, err := language.ParseAcceptLanguage(acceptLanguage)
	if err != nil {
		return false
	}
	resolvedBase, _ := resolved.Base()
	for _, tag := range requested {
		requestedBase, _ := tag.Base()
		if requestedBase == resolvedBase {
			return true
		}
	}
	return false
}
