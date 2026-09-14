package standard

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/status"

	commonpb "github.com/go-sdk/server/tests/pb/common"
)

func TestWithI18nFS(t *testing.T) {
	messageFS := fstest.MapFS{
		"locales/zh-CN.toml": {
			Data: []byte("[\"1000002\"]\nother = \"文件 {{.Name}} 不存在\"\n"),
		},
		"locales/README.txt": {Data: []byte("ignored")},
	}
	cfg := defaultConfig()
	if err := WithI18nFS(messageFS)(&cfg); err != nil {
		t.Fatalf("load i18n file system: %v", err)
	}
	ctx := NewContext(context.Background(), AcceptLanguageKey, "zh-CN")
	err := localizeResponseError(ctx, ErrNotFound.
		WithErrorCode(commonpb.ErrorCode_ERROR_CODE_FILE_NOT_FOUND).
		WithData(map[string]any{"Name": "report.pdf"}), cfg.i18nBundle)

	info := errorInfoFromError(t, err)
	if info.GetReason() != "文件 report.pdf 不存在" {
		t.Fatalf("unexpected localized reason: %q", info.GetReason())
	}
}

func TestWithI18nFSRejectsInvalidInput(t *testing.T) {
	cfg := defaultConfig()
	if err := WithI18nFS(nil)(&cfg); err == nil {
		t.Fatal("expected nil i18n file system error")
	}

	tests := []struct {
		name      string
		messageFS fstest.MapFS
	}{
		{name: "empty", messageFS: fstest.MapFS{}},
		{name: "invalid language", messageFS: fstest.MapFS{"locales/invalid_language.toml": {Data: []byte("message = \"value\"")}}},
		{name: "invalid toml", messageFS: fstest.MapFS{"locales/zh-CN.toml": {Data: []byte("invalid = [")}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := defaultConfig()
			if err := WithI18nFS(test.messageFS)(&cfg); err == nil {
				t.Fatal("expected i18n file system error")
			}
		})
	}
}

func TestLocalizeResponseError(t *testing.T) {
	bundle := i18n.NewBundle(language.English)
	bundle.MustAddMessages(language.SimplifiedChinese, &i18n.Message{
		ID:    "1000002",
		Other: "文件 {{.Name}} 不存在",
	})
	ctx := NewContext(context.Background(), AcceptLanguageKey, "zh-CN")
	err := localizeResponseError(ctx, ErrNotFound.
		WithErrorCode(commonpb.ErrorCode_ERROR_CODE_FILE_NOT_FOUND).
		WithData(map[string]any{"Name": "report.pdf"}), bundle)

	info := errorInfoFromError(t, err)
	if info.GetDomain() != "1000002" || info.GetReason() != "文件 report.pdf 不存在" {
		t.Fatalf("unexpected localized error info: %v", info)
	}
}

func TestLocalizeResponseErrorFallsBackToEnglish(t *testing.T) {
	ctx := NewContext(context.Background(), AcceptLanguageKey, "fr")
	bundle := i18n.NewBundle(language.SimplifiedChinese)
	bundle.MustAddMessages(language.SimplifiedChinese, &i18n.Message{
		ID:    "1000002",
		Other: "文件不存在",
	})
	err := localizeResponseError(ctx, ErrNotFound.
		WithErrorCode(commonpb.ErrorCode_ERROR_CODE_FILE_NOT_FOUND).
		WithData(map[string]any{"Name": "report.pdf"}), bundle)

	info := errorInfoFromError(t, err)
	if info.GetReason() != "file report.pdf not found" {
		t.Fatalf("unexpected English fallback: %q", info.GetReason())
	}
}

func TestLocalizeResponseErrorFallsBackToEnumName(t *testing.T) {
	err := localizeResponseError(context.Background(), ErrInvalidParam.
		WithErrorCode(commonpb.ErrorCode_ERROR_CODE_UNSPECIFIED), i18n.NewBundle(language.English))

	info := errorInfoFromError(t, err)
	if info.GetDomain() != "0" || info.GetReason() != "ERROR_CODE_UNSPECIFIED" {
		t.Fatalf("unexpected enum fallback: %v", info)
	}
}

func TestLocalizeResponseErrorFallsBackToEnumNameWhenTemplateDataMissing(t *testing.T) {
	bundle := i18n.NewBundle(language.English)
	bundle.MustAddMessages(language.SimplifiedChinese, &i18n.Message{
		ID:    "1000002",
		Other: "文件 {{.Name}} 不存在",
	})
	ctx := NewContext(context.Background(), AcceptLanguageKey, "zh-CN")
	err := localizeResponseError(ctx, ErrNotFound.
		WithErrorCode(commonpb.ErrorCode_ERROR_CODE_FILE_NOT_FOUND), bundle)

	info := errorInfoFromError(t, err)
	if info.GetDomain() != "1000002" || info.GetReason() != "ERROR_CODE_FILE_NOT_FOUND" {
		t.Fatalf("unexpected enum fallback: %v", info)
	}
}

func TestLocalizeResponseErrorFallsBackToEnumNameWhenTemplateVariableMissing(t *testing.T) {
	ctx := NewContext(context.Background(), AcceptLanguageKey, "zh-CN")
	err := localizeResponseError(ctx, ErrNotFound.
		WithErrorCode(commonpb.ErrorCode_ERROR_CODE_FILE_NOT_FOUND).
		WithData(map[string]any{"Other": "x"}), i18n.NewBundle(language.English))

	info := errorInfoFromError(t, err)
	if info.GetDomain() != "1000002" || info.GetReason() != "ERROR_CODE_FILE_NOT_FOUND" {
		t.Fatalf("unexpected enum fallback: %v", info)
	}
}

func errorInfoFromError(t *testing.T, err error) *errdetails.ErrorInfo {
	t.Helper()
	details := status.Convert(err).Details()
	if len(details) == 0 {
		t.Fatal("expected error info")
	}
	info, ok := details[0].(*errdetails.ErrorInfo)
	if !ok {
		t.Fatalf("unexpected error detail: %T", details[0])
	}
	return info
}
