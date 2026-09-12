package main

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
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
func serveHandler(handler func(http.ResponseWriter, *http.Request, map[string]string), request *http.Request, pathParams map[string]string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler(recorder, request, pathParams)
	return recorder
}

func TestFileStoreUpload(t *testing.T) {
	store := newFileStore()

	t.Run("stores file and returns metadata", func(t *testing.T) {
		content := []byte("hello, files")
		response := serveHandler(store.handleUpload, newUploadRequest(t, "hello.txt", content), nil)
		if response.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d, body: %s", response.Code, response.Body.String())
		}
		if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
			t.Fatalf("unexpected content type: %s", contentType)
		}
		body := response.Body.String()
		if !strings.Contains(body, `"name":"hello.txt"`) {
			t.Fatalf("unexpected name in response: %s", body)
		}
		if !strings.Contains(body, `"size":12`) {
			t.Fatalf("unexpected size in response: %s", body)
		}
	})

	t.Run("rejects missing file field", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("not a multipart form"))
		response := serveHandler(store.handleUpload, request, nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("unexpected status: %d", response.Code)
		}
	})

	t.Run("rejects file exceeding size limit", func(t *testing.T) {
		content := make([]byte, maxFileSize+1)
		response := serveHandler(store.handleUpload, newUploadRequest(t, "large.bin", content), nil)
		if response.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("unexpected status: %d", response.Code)
		}
	})

	t.Run("sanitizes path traversal in filename", func(t *testing.T) {
		response := serveHandler(store.handleUpload, newUploadRequest(t, "../../etc/passwd", []byte("x")), nil)
		if response.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", response.Code)
		}
		if !strings.Contains(response.Body.String(), `"name":"passwd"`) {
			t.Fatalf("unexpected name in response: %s", response.Body.String())
		}
	})

	t.Run("overwrites existing file with same name", func(t *testing.T) {
		serveHandler(store.handleUpload, newUploadRequest(t, "dup.txt", []byte("first")), nil)
		serveHandler(store.handleUpload, newUploadRequest(t, "dup.txt", []byte("second version")), nil)
		response := serveHandler(store.handleDownload, httptest.NewRequest(http.MethodGet, "/download/dup.txt", nil),
			map[string]string{"name": "dup.txt"})
		if response.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", response.Code)
		}
		if response.Body.String() != "second version" {
			t.Fatalf("unexpected content: %q", response.Body.String())
		}
	})
}

func TestFileStoreDownload(t *testing.T) {
	store := newFileStore()
	if response := serveHandler(store.handleUpload, newUploadRequest(t, "hello.txt", []byte("hello, files")), nil); response.Code != http.StatusOK {
		t.Fatalf("upload file: status %d", response.Code)
	}

	t.Run("returns file content as attachment", func(t *testing.T) {
		response := serveHandler(store.handleDownload, httptest.NewRequest(http.MethodGet, "/download/hello.txt", nil),
			map[string]string{"name": "hello.txt"})
		if response.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", response.Code)
		}
		if response.Body.String() != "hello, files" {
			t.Fatalf("unexpected content: %q", response.Body.String())
		}
		if contentType := response.Header().Get("Content-Type"); contentType != "application/octet-stream" {
			t.Fatalf("unexpected content type: %s", contentType)
		}
		if disposition := response.Header().Get("Content-Disposition"); disposition != `attachment; filename="hello.txt"` {
			t.Fatalf("unexpected disposition: %s", disposition)
		}
		if length := response.Header().Get("Content-Length"); length != "12" {
			t.Fatalf("unexpected content length: %s", length)
		}
	})

	t.Run("responds not found for missing file", func(t *testing.T) {
		response := serveHandler(store.handleDownload, httptest.NewRequest(http.MethodGet, "/download/missing.txt", nil),
			map[string]string{"name": "missing.txt"})
		if response.Code != http.StatusNotFound {
			t.Fatalf("unexpected status: %d", response.Code)
		}
	})

	t.Run("sanitizes path traversal in route parameter", func(t *testing.T) {
		// 先以穿越路径之外的正常文件名上传
		if response := serveHandler(store.handleUpload, newUploadRequest(t, "secret.txt", []byte("s")), nil); response.Code != http.StatusOK {
			t.Fatalf("upload file: status %d", response.Code)
		}
		response := serveHandler(store.handleDownload, httptest.NewRequest(http.MethodGet, "/download/secret.txt", nil),
			map[string]string{"name": filepath.Join("..", "..", "secret.txt")})
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
	done := make(chan struct{})
	for i := range 8 {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			response := serveHandler(store.handleUpload, newUploadRequest(t, "file.txt", []byte{byte(i)}), nil)
			if response.Code != http.StatusOK {
				t.Errorf("upload file: status %d", response.Code)
				return
			}
			response = serveHandler(store.handleDownload, httptest.NewRequest(http.MethodGet, "/download/file.txt", nil),
				map[string]string{"name": "file.txt"})
			if response.Code != http.StatusOK {
				t.Errorf("download file: status %d", response.Code)
			}
		}(i)
	}
	for range 8 {
		<-done
	}
}
