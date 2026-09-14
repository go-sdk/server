package main

import (
	"bytes"
	"encoding/json"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/go-sdk/server/standard"
)

// newUploadRequest 构造字段名为 file 的 multipart 上传请求。
func newUploadRequest(t *testing.T, filename string, content []byte) *http.Request {
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

// serveHandler 以给定路径参数执行 Handler 并返回响应记录。
func serveHandler(handler standard.HandlerFunc, request *http.Request, pathParams map[string]string) (*httptest.ResponseRecorder, error) {
	recorder := httptest.NewRecorder()
	err := handler(standard.NewHTTPContext(request, recorder, pathParams))
	return recorder, err
}

func TestFileStoreUpload(t *testing.T) {
	store := newFileStore()

	t.Run("stores file and returns metadata", func(t *testing.T) {
		content := []byte("hello, files")
		response, err := serveHandler(store.handleUpload, newUploadRequest(t, "hello.txt", content), nil)
		if err != nil {
			t.Fatalf("upload file: %v", err)
		}
		if response.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d, body: %s", response.Code, response.Body.String())
		}
		if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
			t.Fatalf("unexpected content type: %s", contentType)
		}
		var metadata struct {
			Name string `json:"name"`
			Size int    `json:"size"`
		}
		if err = json.NewDecoder(response.Body).Decode(&metadata); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if metadata.Name != "hello.txt" || metadata.Size != len(content) {
			t.Fatalf("unexpected metadata: %+v", metadata)
		}
	})

	t.Run("rejects missing file field", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("not a multipart form"))
		_, err := serveHandler(store.handleUpload, request, nil)
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rejects file exceeding size limit", func(t *testing.T) {
		content := make([]byte, maxFileSize+1)
		_, err := serveHandler(store.handleUpload, newUploadRequest(t, "large.bin", content), nil)
		if status.Code(err) != codes.ResourceExhausted {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("sanitizes path traversal in filename", func(t *testing.T) {
		response, err := serveHandler(store.handleUpload, newUploadRequest(t, "../../etc/passwd", []byte("x")), nil)
		if err != nil {
			t.Fatalf("upload file: %v", err)
		}
		if response.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", response.Code)
		}
		if !strings.Contains(response.Body.String(), `"name":"passwd"`) {
			t.Fatalf("unexpected name in response: %s", response.Body.String())
		}
	})

	t.Run("overwrites existing file with same name", func(t *testing.T) {
		if _, err := serveHandler(store.handleUpload, newUploadRequest(t, "dup.txt", []byte("first")), nil); err != nil {
			t.Fatalf("upload first file: %v", err)
		}
		if _, err := serveHandler(store.handleUpload, newUploadRequest(t, "dup.txt", []byte("second version")), nil); err != nil {
			t.Fatalf("upload second file: %v", err)
		}
		response, err := serveHandler(store.handleDownload, httptest.NewRequest(http.MethodGet, "/download/dup.txt", nil),
			map[string]string{"name": "dup.txt"})
		if err != nil {
			t.Fatalf("download file: %v", err)
		}
		if response.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", response.Code)
		}
		if response.Body.String() != "second version" {
			t.Fatalf("unexpected content: %q", response.Body.String())
		}
	})
}

func TestNormalizeFileName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
		ok    bool
	}{
		{name: "plain", value: "hello.txt", want: "hello.txt", ok: true},
		{name: "unix traversal", value: "../../secret.txt", want: "secret.txt", ok: true},
		{name: "windows traversal", value: `..\..\secret.txt`, want: "secret.txt", ok: true},
		{name: "empty", value: "", ok: false},
		{name: "current directory", value: ".", ok: false},
		{name: "parent directory", value: "..", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeFileName(tt.value)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("normalizeFileName(%q) = %q, %t; want %q, %t", tt.value, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestFileStoreDownload(t *testing.T) {
	store := newFileStore()
	if response, err := serveHandler(store.handleUpload, newUploadRequest(t, "hello.txt", []byte("hello, files")), nil); err != nil || response.Code != http.StatusOK {
		t.Fatalf("upload file: status %d", response.Code)
	}

	t.Run("returns file content as attachment", func(t *testing.T) {
		response, err := serveHandler(store.handleDownload, httptest.NewRequest(http.MethodGet, "/download/hello.txt", nil),
			map[string]string{"name": "hello.txt"})
		if err != nil {
			t.Fatalf("download file: %v", err)
		}
		if response.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", response.Code)
		}
		if response.Body.String() != "hello, files" {
			t.Fatalf("unexpected content: %q", response.Body.String())
		}
		if contentType := response.Header().Get("Content-Type"); contentType != "application/octet-stream" {
			t.Fatalf("unexpected content type: %s", contentType)
		}
		disposition, params, err := mime.ParseMediaType(response.Header().Get("Content-Disposition"))
		if err != nil {
			t.Fatalf("parse disposition: %v", err)
		}
		if disposition != "attachment" || params["filename"] != "hello.txt" {
			t.Fatalf("unexpected disposition: %s, params: %v", disposition, params)
		}
		if length := response.Header().Get("Content-Length"); length != "12" {
			t.Fatalf("unexpected content length: %s", length)
		}
	})

	t.Run("responds not found for missing file", func(t *testing.T) {
		_, err := serveHandler(store.handleDownload, httptest.NewRequest(http.MethodGet, "/download/missing.txt", nil),
			map[string]string{"name": "missing.txt"})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("sanitizes path traversal in route parameter", func(t *testing.T) {
		// 先以穿越路径之外的正常文件名上传
		if response, err := serveHandler(store.handleUpload, newUploadRequest(t, "secret.txt", []byte("s")), nil); err != nil || response.Code != http.StatusOK {
			t.Fatalf("upload file: status %d", response.Code)
		}
		response, err := serveHandler(store.handleDownload, httptest.NewRequest(http.MethodGet, "/download/secret.txt", nil),
			map[string]string{"name": filepath.Join("..", "..", "secret.txt")})
		if err != nil {
			t.Fatalf("download file: %v", err)
		}
		if response.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", response.Code)
		}
		if response.Body.String() != "s" {
			t.Fatalf("unexpected content: %q", response.Body.String())
		}
	})
}

func TestFileStoreConcurrentAccess(t *testing.T) {
	store := newFileStore()
	requests := make([]*http.Request, 8)
	for i := range requests {
		requests[i] = newUploadRequest(t, "file.txt", []byte{byte(i)})
	}

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			response, err := serveHandler(store.handleUpload, requests[i], nil)
			if err != nil {
				t.Errorf("upload file: %v", err)
				return
			}
			if response.Code != http.StatusOK {
				t.Errorf("upload file: status %d", response.Code)
				return
			}
			response, err = serveHandler(store.handleDownload, httptest.NewRequest(http.MethodGet, "/download/file.txt", nil),
				map[string]string{"name": "file.txt"})
			if err != nil {
				t.Errorf("download file: %v", err)
				return
			}
			if response.Code != http.StatusOK {
				t.Errorf("download file: status %d", response.Code)
			}
		}(i)
	}
	wg.Wait()
}
