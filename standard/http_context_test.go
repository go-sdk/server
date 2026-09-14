package standard

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newMultipartRequest(t *testing.T, filename string, content []byte) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err = part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err = writer.Close(); err != nil {
		t.Fatalf("close form writer: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/upload", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func TestHTTPContextRequestHelpers(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/files/report.txt?tag=first&tag=second", nil)
	request.Header.Set("X-Test", "header-value")
	recorder := httptest.NewRecorder()
	ctx := NewHTTPContext(request, recorder, map[string]string{"name": "report.txt"})

	if ctx.Request() != request || ctx.Response() != recorder {
		t.Fatal("http context must expose the original request and response")
	}
	if ctx.Param("name") != "report.txt" {
		t.Fatalf("unexpected path parameter: %q", ctx.Param("name"))
	}
	if ctx.Query("tag") != "first" {
		t.Fatalf("unexpected query value: %q", ctx.Query("tag"))
	}
	if values := ctx.Queries()["tag"]; len(values) != 2 || values[1] != "second" {
		t.Fatalf("unexpected query values: %v", values)
	}
	if ctx.Header("X-Test") != "header-value" {
		t.Fatalf("unexpected header value: %q", ctx.Header("X-Test"))
	}
}

func TestHTTPContextResponseHelpers(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx := NewHTTPContext(httptest.NewRequest(http.MethodGet, "/", nil), recorder, nil)
	ctx.SetHeader("X-Test", "response-value")
	if err := ctx.JSON(http.StatusCreated, map[string]string{"name": "report.txt"}); err != nil {
		t.Fatalf("write json response: %v", err)
	}

	if recorder.Code != http.StatusCreated {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if recorder.Header().Get("X-Test") != "response-value" {
		t.Fatalf("unexpected response header: %q", recorder.Header().Get("X-Test"))
	}
	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected content type: %q", recorder.Header().Get("Content-Type"))
	}
	if recorder.Body.String() != "{\"name\":\"report.txt\"}\n" {
		t.Fatalf("unexpected response body: %q", recorder.Body.String())
	}
}

func TestContextWithoutHTTPRequest(t *testing.T) {
	ctx := FromContext(context.Background())
	if ctx.Request() != nil || ctx.Response() != nil {
		t.Fatal("non-http context must not expose request or response")
	}
	if ctx.Param("name") != "" || ctx.Query("name") != "" || ctx.Header("name") != "" {
		t.Fatal("non-http context helpers must return empty values")
	}
}

func TestHTTPContextReadFormFileLimitsFileContent(t *testing.T) {
	t.Run("accepts file at limit", func(t *testing.T) {
		request := newMultipartRequest(t, "report.txt", []byte("data"))
		ctx := NewHTTPContext(request, httptest.NewRecorder(), nil)
		filename, data, err := ctx.ReadFormFile("file", 4)
		if err != nil {
			t.Fatalf("read form file: %v", err)
		}
		if filename != "report.txt" || string(data) != "data" {
			t.Fatalf("unexpected file: name=%q data=%q", filename, data)
		}
	})

	t.Run("rejects file over limit", func(t *testing.T) {
		request := newMultipartRequest(t, "report.txt", []byte("large"))
		ctx := NewHTTPContext(request, httptest.NewRecorder(), nil)
		_, _, err := ctx.ReadFormFile("file", 4)
		if status.Code(err) != codes.ResourceExhausted {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
