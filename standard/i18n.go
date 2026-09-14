package standard

import (
	"context"
	"io/fs"
	"path"
	"strings"

	"github.com/BurntSushi/toml"
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

// WithI18nFS 从文件系统递归加载 TOML 翻译文件，并使用英文作为默认语言。
func WithI18nFS(messageFS fs.FS) Option {
	return func(c *config) error {
		bundle, err := loadI18nBundle(messageFS)
		if err != nil {
			return err
		}
		c.i18nBundle = bundle
		return nil
	}
}

func loadI18nBundle(messageFS fs.FS) (*i18n.Bundle, error) {
	if messageFS == nil {
		return nil, errx.New("i18n file system must not be nil")
	}
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	fileCount := 0
	err := fs.WalkDir(messageFS, ".", func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errx.Wrapf(walkErr, "walk i18n path %s", filePath)
		}
		if entry.IsDir() || path.Ext(filePath) != ".toml" {
			return nil
		}
		if err := validateI18nFileName(filePath); err != nil {
			return err
		}
		if _, loadErr := bundle.LoadMessageFileFS(messageFS, filePath); loadErr != nil {
			return errx.Wrapf(loadErr, "load i18n message file %s", filePath)
		}
		fileCount++
		return nil
	})
	if err != nil {
		return nil, err
	}
	if fileCount == 0 {
		return nil, errx.New("i18n file system must contain at least one toml file")
	}
	return bundle, nil
}

func validateI18nFileName(filePath string) error {
	name := strings.TrimSuffix(path.Base(filePath), path.Ext(filePath))
	if index := strings.LastIndexByte(name, '.'); index >= 0 {
		name = name[index+1:]
	}
	if _, err := language.Parse(name); err != nil {
		return errx.Wrapf(err, "parse i18n language from file %s", filePath)
	}
	return nil
}

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
