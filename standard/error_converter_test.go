package standard

import (
	"context"
	"database/sql"
	"testing"

	"github.com/go-sdk/core/errx"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestErrorConverterConvertsWrappedError(t *testing.T) {
	converter := ErrorConvertFunc(func(err error) (RespError, bool) {
		if !errx.Is(err, sql.ErrNoRows) {
			return RespError{}, false
		}
		return ErrNotFound.WithMessage("record not found").
			WithDomainReason("RECORD_NOT_FOUND", "record.not_found"), true
	})

	err := convertResponseError(errx.Wrap(sql.ErrNoRows, "query user"), []ErrorConverter{converter})
	grpcStatus := status.Convert(err)
	if grpcStatus.Code() != codes.NotFound || grpcStatus.Message() != "record not found" {
		t.Fatalf("unexpected converted status: %v", grpcStatus)
	}
	info, ok := grpcStatus.Details()[0].(*errdetails.ErrorInfo)
	if !ok {
		t.Fatalf("unexpected error info: %T", grpcStatus.Details()[0])
	}
	if info.GetDomain() != "RECORD_NOT_FOUND" || info.GetReason() != "record.not_found" {
		t.Fatalf("unexpected domain and reason: %v", info)
	}
}

func TestErrorConverterUsesFirstMatch(t *testing.T) {
	first := ErrorConvertFunc(func(error) (RespError, bool) {
		return ErrNotFound.WithMessage("first"), true
	})
	secondCalled := false
	second := ErrorConvertFunc(func(error) (RespError, bool) {
		secondCalled = true
		return ErrInternal.WithMessage("second"), true
	})

	err := convertResponseError(sql.ErrNoRows, []ErrorConverter{first, second})
	if status.Convert(err).Message() != "first" {
		t.Fatalf("unexpected converted error: %v", err)
	}
	if secondCalled {
		t.Fatal("converter evaluation must stop after the first match")
	}
}

func TestErrorConverterKeepsUnmatchedError(t *testing.T) {
	original := errx.New("database unavailable")
	converter := ErrorConvertFunc(func(error) (RespError, bool) {
		return RespError{}, false
	})
	if converted := convertResponseError(original, []ErrorConverter{converter}); converted != original {
		t.Fatalf("unmatched error must remain unchanged: %v", converted)
	}
	if converted := convertResponseError(nil, []ErrorConverter{converter}); converted != nil {
		t.Fatalf("nil error must remain nil: %v", converted)
	}
}

func TestUnaryErrorConverterInterceptor(t *testing.T) {
	converter := ErrorConvertFunc(func(err error) (RespError, bool) {
		if errx.Is(err, sql.ErrNoRows) {
			return ErrInvalidParam, true
		}
		return RespError{}, false
	})
	interceptor := unaryErrorConverterInterceptor([]ErrorConverter{converter}, nil)
	response, err := interceptor(
		context.Background(),
		nil,
		&grpc.UnaryServerInfo{},
		func(context.Context, any) (any, error) {
			return "response", sql.ErrNoRows
		},
	)
	if response != "response" || status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unexpected interceptor result: response=%v error=%v", response, err)
	}
}

func TestStreamErrorConverterInterceptor(t *testing.T) {
	converter := ErrorConvertFunc(func(err error) (RespError, bool) {
		if errx.Is(err, sql.ErrNoRows) {
			return ErrNotFound.WithMessage("record not found"), true
		}
		return RespError{}, false
	})
	interceptor := streamErrorConverterInterceptor([]ErrorConverter{converter}, nil)
	stream := &contextServerStream{ctx: context.Background()}
	err := interceptor(nil, stream, &grpc.StreamServerInfo{}, func(any, grpc.ServerStream) error {
		return sql.ErrNoRows
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("unexpected stream interceptor error: %v", err)
	}
}

func TestWithErrorConvertersRejectsNil(t *testing.T) {
	cfg := defaultConfig()
	if err := WithErrorConverters(nil)(&cfg); err == nil {
		t.Fatal("expected nil converter error")
	}
	if err := WithErrorConverters(ErrorConvertFunc(nil))(&cfg); err == nil {
		t.Fatal("expected typed nil converter error")
	}
}
