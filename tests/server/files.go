package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/go-sdk/core/errx"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"github.com/go-sdk/server/standard"
)

// maxFileSize 限制单次上传大小，避免示例服务占用过多内存
const maxFileSize = 32 << 20

// fileStore 以内存方式保存上传文件，仅用于演示 HandlePath 注册的上传和下载接口。
type fileStore struct {
	mu    sync.RWMutex
	files map[string][]byte
}

func newFileStore() *fileStore {
	return &fileStore{files: make(map[string][]byte)}
}

// handleUpload 处理 POST /upload，multipart 表单字段名为 file。
func (s *fileStore) handleUpload(w http.ResponseWriter, r *http.Request, _ map[string]string) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFileSize)
	if err := r.ParseMultipartForm(maxFileSize); err != nil {
		status := http.StatusBadRequest
		var maxErr *http.MaxBytesError
		if errx.As(err, &maxErr) {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, "parse multipart form", status)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()

	// 只保留文件名，防止路径穿越
	name := filepath.Base(header.Filename)
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "read file content", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.files[name] = data
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	response := struct {
		Name string `json:"name"`
		Size int    `json:"size"`
	}{Name: name, Size: len(data)}
	if err = json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
	}
}

// handleDownload 处理 GET /download/{name}，以附件形式返回文件内容。
func (s *fileStore) handleDownload(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
	name := filepath.Base(pathParams["name"])

	s.mu.RLock()
	data, ok := s.files[name]
	s.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if _, err := w.Write(data); err != nil {
		http.Error(w, "write file content", http.StatusInternalServerError)
	}
}

// registerFileHandlers 将上传和下载接口注册到额外 HTTP 路由。
func (s *fileStore) registerFileHandlers(server *standard.Server) error {
	if err := server.HandlePath(http.MethodPost, "/upload", s.handleUpload); err != nil {
		return errx.Wrap(err, "register upload route")
	}
	if err := server.HandlePath(http.MethodGet, "/download/{name}", s.handleDownload); err != nil {
		return errx.Wrap(err, "register download route")
	}
	return nil
}

var (
	_ runtime.HandlerFunc = (*fileStore)(nil).handleUpload
	_ runtime.HandlerFunc = (*fileStore)(nil).handleDownload
)
