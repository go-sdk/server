package standard

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-sdk/server/jwtx"
)

func TestHTTPRouteHandlerRendersRespError(t *testing.T) {
	server := &Server{}
	handler := server.httpRouteHandler(func(*Context) error {
		return ErrInvalidParam.WithDomainReason("FILE_1001", "file.name.required")
	})
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/files", nil), nil)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	want := "{\"code\":3,\"message\":\"invalid parameter\",\"domain\":\"FILE_1001\",\"reason\":\"file.name.required\",\"details\":[]}\n"
	if recorder.Body.String() != want {
		t.Fatalf("unexpected response body:\n got: %s\nwant: %s", recorder.Body.String(), want)
	}
}

func TestHTTPRouteHandlerConvertsKnownError(t *testing.T) {
	sourceErr := errors.New("record missing")
	server := &Server{config: config{errorConverters: []ErrorConverter{
		ErrorConvertFunc(func(err error) (RespError, bool) {
			return ErrNotFound.WithMessage("record not found"), errors.Is(err, sourceErr)
		}),
	}}}
	handler := server.httpRouteHandler(func(*Context) error { return sourceErr })
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/records/1", nil), nil)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if recorder.Body.String() != "{\"code\":5,\"message\":\"record not found\",\"domain\":\"\",\"reason\":\"\",\"details\":[]}\n" {
		t.Fatalf("unexpected response body: %s", recorder.Body.String())
	}
}

func TestHTTPRouteHandlerUsesExplicitHTTPStatus(t *testing.T) {
	server := &Server{}
	handler := server.httpRouteHandler(func(*Context) error {
		return ErrResourceExhausted.
			WithMessage("file is too large").
			WithHTTPStatus(http.StatusRequestEntityTooLarge)
	})
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodPost, "/upload", nil), nil)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
}

func TestHTTPRouteHandlerHidesUnknownError(t *testing.T) {
	server := &Server{}
	handler := server.httpRouteHandler(func(*Context) error { return errors.New("database password leaked") })
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/records", nil), nil)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if recorder.Body.String() != "{\"code\":13,\"message\":\"internal server error\",\"domain\":\"\",\"reason\":\"\",\"details\":[]}\n" {
		t.Fatalf("unexpected response body: %s", recorder.Body.String())
	}
}

func TestHTTPRouteHandlerRequiresJWT(t *testing.T) {
	codec := jwtx.HS256("secret")
	server := &Server{config: config{jwtAuth: &codec.Parser}}
	handler := server.httpRouteHandler(func(*Context) error {
		t.Fatal("handler must not run without a valid token")
		return nil
	})
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/private", nil), nil)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if recorder.Body.String() != "{\"code\":16,\"message\":\"invalid bearer token\",\"domain\":\"\",\"reason\":\"\",\"details\":[]}\n" {
		t.Fatalf("unexpected response body: %s", recorder.Body.String())
	}
}

func TestHTTPRouteHandlerRecoversPanic(t *testing.T) {
	server := &Server{}
	handler := server.httpRouteHandler(func(*Context) error { panic("failed") })
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil), nil)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
}

func TestHTTPRouteHandlerDoesNotOverwriteCommittedResponse(t *testing.T) {
	server := &Server{}
	handler := server.httpRouteHandler(func(ctx *Context) error {
		if err := ctx.Text(http.StatusAccepted, "accepted"); err != nil {
			return err
		}
		return ErrInternal
	})
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/committed", nil), nil)

	if recorder.Code != http.StatusAccepted || recorder.Body.String() != "accepted" {
		t.Fatalf("unexpected committed response: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}
