package testserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc/codes"

	"github.com/go-sdk/server/standard"
)

func TestNewHTTP(t *testing.T) {
	type requestValues struct {
		Path   string
		Query  string
		Header string
	}
	values := make(chan requestValues, 1)
	server := NewHTTP(t, http.MethodGet, "/files/{id}", func(c *standard.Context) error {
		values <- requestValues{
			Path:   c.Param("id"),
			Query:  c.Query("download"),
			Header: c.Header("X-Test-Header"),
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	request, err := http.NewRequest(http.MethodGet, server.URL("/files/42?download=true"), nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("X-Test-Header", "value")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: got %d, want %d", response.StatusCode, http.StatusOK)
	}
	var body map[string]string
	if err = json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("unexpected response: %#v", body)
	}
	got := <-values
	want := requestValues{Path: "42", Query: "true", Header: "value"}
	if got != want {
		t.Fatalf("unexpected request values: got %#v, want %#v", got, want)
	}
}

func TestNewHTTPConvertsHandlerError(t *testing.T) {
	sourceErr := errors.New("source error")
	server := NewHTTP(
		t,
		http.MethodGet,
		"/failure",
		func(*standard.Context) error { return fmt.Errorf("load resource: %w", sourceErr) },
		standard.WithErrorConverters(standard.ErrorConvertFunc(func(err error) (standard.RespError, bool) {
			if !errors.Is(err, sourceErr) {
				return standard.RespError{}, false
			}
			return standard.ErrNotFound.WithMessage("converted error"), true
		})),
	)
	response, err := server.Client().Get(server.URL("/failure"))
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected status: got %d, want %d", response.StatusCode, http.StatusNotFound)
	}
	var body struct {
		Code    int32  `json:"code"`
		Message string `json:"message"`
	}
	if err = json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != int32(codes.NotFound) || body.Message != "converted error" {
		t.Fatalf("unexpected response: %#v", body)
	}
	server.Close()
	server.Close()
}

func TestNewHTTPUsesStandardAuthentication(t *testing.T) {
	var called atomic.Bool
	server := NewHTTP(
		t,
		http.MethodGet,
		"/protected",
		func(c *standard.Context) error {
			called.Store(true)
			return c.NoContent(http.StatusNoContent)
		},
		standard.WithJWTSecret([]byte("test-secret")),
	)
	response, err := server.Client().Get(server.URL("/protected"))
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected status: got %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
	if called.Load() {
		t.Fatal("handler must not run without a valid bearer token")
	}
}
