package standard

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-sdk/core/errx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"github.com/go-sdk/server/jwtx"
)

func TestHandlePathSkipAuthIsRouteLocal(t *testing.T) {
	codec := jwtx.HS256("test-secret")
	server := &Server{config: config{jwtAuth: &codec.Parser}, gatewayMux: runtime.NewServeMux()}
	const body = "{ \"op\": 13, \"d\": {} }"
	if err := server.HandlePath(http.MethodPost, "/public", func(c *Context) error {
		if len(c.JWT()) != 0 {
			t.Fatal("public route must not receive jwt claims")
		}
		if c.Header("X-Signature-Timestamp") != "123" {
			t.Fatal("signature header was not preserved")
		}
		data, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		if string(data) != body {
			t.Fatal("raw request body was not preserved")
		}
		return c.Blob(http.StatusOK, "application/json", data)
	}, WithRouteSkipAuth()); err != nil {
		t.Fatal(err)
	}
	if err := server.HandlePath(http.MethodGet, "/private", func(c *Context) error {
		if c.JWT()["sub"] != "user-1" {
			t.Fatal("private route must receive verified claims")
		}
		return c.NoContent(http.StatusNoContent)
	}); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/public", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer invalid")
	request.Header.Set("X-Signature-Timestamp", "123")
	recorder := httptest.NewRecorder()
	server.gatewayMux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != body {
		t.Fatal("public route must preserve the raw response without jwt validation")
	}

	recorder = httptest.NewRecorder()
	server.gatewayMux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/private", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("private route without jwt returned status %d", recorder.Code)
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "user-1"}).SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/private", nil)
	request.Header.Set("Authorization", "Bearer "+signed)
	recorder = httptest.NewRecorder()
	server.gatewayMux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("private route with jwt returned status %d", recorder.Code)
	}
}

func TestHandlePathRejectsInvalidRouteOptions(t *testing.T) {
	sourceErr := errx.New("invalid route configuration")
	for _, test := range []struct {
		name   string
		option RouteOption
	}{
		{name: "nil"},
		{name: "error", option: func(*routeConfig) error { return sourceErr }},
	} {
		t.Run(test.name, func(t *testing.T) {
			codec := jwtx.HS256("test-secret")
			server := &Server{config: config{jwtAuth: &codec.Parser}, gatewayMux: runtime.NewServeMux()}
			handler := func(*Context) error {
				t.Fatal("rejected route must not bypass jwt")
				return nil
			}
			err := server.HandlePath(http.MethodGet, "/private", handler, WithRouteSkipAuth(), test.option)
			if err == nil {
				t.Fatal("expected invalid route option error")
			}
			if test.option != nil && !errx.Is(err, sourceErr) {
				t.Fatal("route option error was not preserved")
			}
			if err := server.HandlePath(http.MethodGet, "/private", handler); err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			server.gatewayMux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/private", nil))
			if recorder.Code != http.StatusUnauthorized {
				t.Fatal("failed route options must not affect later registrations")
			}
		})
	}
}
