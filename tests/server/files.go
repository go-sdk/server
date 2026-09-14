package main

import (
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/go-sdk/core/errx"

	"github.com/go-sdk/server/standard"
	commonpb "github.com/go-sdk/server/tests/pb/common"
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

// normalizeFileName 只保留文件名，并拒绝空名和目录特殊名称。
func normalizeFileName(value string) (string, bool) {
	name := filepath.Base(strings.ReplaceAll(value, `\`, "/"))
	if name == "" || name == "." || name == ".." || name == string(filepath.Separator) {
		return "", false
	}
	return name, true
}

// handleUpload 处理 POST /upload，multipart 表单字段名为 file。
func (s *fileStore) handleUpload(c *standard.Context) error {
	filename, data, err := c.ReadFormFile("file", maxFileSize)
	if err != nil {
		return err
	}

	name, ok := normalizeFileName(filename)
	if !ok {
		return standard.ErrInvalidParam.
			WithErrorCode(commonpb.ErrorCode_ERROR_CODE_INVALID_FILE_NAME)
	}
	s.mu.Lock()
	s.files[name] = data
	s.mu.Unlock()

	response := struct {
		Name string `json:"name"`
		Size int    `json:"size"`
	}{Name: name, Size: len(data)}
	return c.JSON(http.StatusOK, response)
}

// handleDownload 处理 GET /download/{name}，以附件形式返回文件内容。
func (s *fileStore) handleDownload(c *standard.Context) error {
	name, valid := normalizeFileName(c.Param("name"))
	if !valid {
		return standard.ErrInvalidParam.
			WithErrorCode(commonpb.ErrorCode_ERROR_CODE_INVALID_FILE_NAME)
	}

	s.mu.RLock()
	data, ok := s.files[name]
	s.mu.RUnlock()
	if !ok {
		return standard.ErrNotFound.
			WithErrorCode(commonpb.ErrorCode_ERROR_CODE_FILE_NOT_FOUND).
			WithData(map[string]any{"Name": name})
	}

	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	if disposition == "" {
		return standard.ErrInvalidParam.
			WithErrorCode(commonpb.ErrorCode_ERROR_CODE_INVALID_FILE_NAME)
	}
	c.SetHeader("Content-Disposition", disposition)
	c.SetHeader("Content-Length", strconv.Itoa(len(data)))
	return c.Blob(http.StatusOK, "application/octet-stream", data)
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

var _ standard.HandlerFunc = (*fileStore)(nil).handleUpload
var _ standard.HandlerFunc = (*fileStore)(nil).handleDownload
