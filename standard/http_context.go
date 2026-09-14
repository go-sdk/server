package standard

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-sdk/core/errx"
)

const (
	multipartFormMemory      int64 = 1 << 20
	multipartRequestOverhead int64 = 1 << 20
)

// HandlerFunc 处理通过 Server.HandlePath 注册的额外 HTTP 请求；返回的错误由 Server 统一转换并写入响应。
type HandlerFunc func(c *Context) error

type httpContext struct {
	request    *http.Request
	response   http.ResponseWriter
	pathParams map[string]string
	committed  bool
}

// NewHTTPContext 创建包含原始 HTTP 请求、响应和路径参数的 Context，主要用于测试 Handler。
func NewHTTPContext(request *http.Request, response http.ResponseWriter, pathParams map[string]string) *Context {
	ctx := context.Background()
	if request != nil {
		ctx = request.Context()
	}
	current := FromContext(ctx)
	return &Context{
		Context:       ctx,
		values:        maps.Clone(current.values),
		authorization: current.authorization,
		http: &httpContext{
			request:    request,
			response:   response,
			pathParams: maps.Clone(pathParams),
		},
	}
}

// Request 返回额外 HTTP Handler 的原始请求；非 HTTP Context 返回 nil。
func (c *Context) Request() *http.Request {
	if c == nil || c.http == nil {
		return nil
	}
	return c.http.request
}

// Response 返回额外 HTTP Handler 的原始响应写入器；非 HTTP Context 返回 nil。
func (c *Context) Response() http.ResponseWriter {
	if c == nil || c.http == nil {
		return nil
	}
	return c.http.response
}

// Param 返回路由中的命名路径参数。
func (c *Context) Param(name string) string {
	if c == nil || c.http == nil {
		return ""
	}
	return c.http.pathParams[name]
}

// Query 返回查询参数的第一个值。
func (c *Context) Query(name string) string {
	request := c.Request()
	if request == nil {
		return ""
	}
	return request.URL.Query().Get(name)
}

// Queries 返回全部查询参数。
func (c *Context) Queries() url.Values {
	request := c.Request()
	if request == nil {
		return url.Values{}
	}
	return request.URL.Query()
}

// Header 返回请求头的第一个值。
func (c *Context) Header(name string) string {
	request := c.Request()
	if request == nil {
		return ""
	}
	return request.Header.Get(name)
}

// FormValue 返回表单字段的第一个值。
func (c *Context) FormValue(name string) string {
	request := c.Request()
	if request == nil {
		return ""
	}
	return request.FormValue(name)
}

// FormFile 返回 multipart 表单中的第一个文件。
func (c *Context) FormFile(name string) (*multipart.FileHeader, error) {
	request := c.Request()
	if request == nil {
		return nil, errx.New("http request is unavailable")
	}
	file, header, err := request.FormFile(name)
	if err != nil {
		return nil, err
	}
	if err = file.Close(); err != nil {
		return nil, errx.Wrap(err, "close multipart file")
	}
	return header, nil
}

// ReadFormFile 在限制文件和请求体大小后读取 multipart 表单中的第一个文件。
func (c *Context) ReadFormFile(name string, maxFileBytes int64) (string, []byte, error) {
	request := c.Request()
	if request == nil {
		return "", nil, errx.New("http request is unavailable")
	}
	response := c.Response()
	if response == nil {
		return "", nil, errx.New("http response is unavailable")
	}
	if maxFileBytes <= 0 {
		return "", nil, errx.New("multipart max file bytes must be positive")
	}
	maxRequestBytes := maxFileBytes + multipartRequestOverhead
	if maxRequestBytes < maxFileBytes {
		return "", nil, errx.New("multipart max file bytes is too large")
	}
	request.Body = http.MaxBytesReader(response, request.Body, maxRequestBytes)
	if err := request.ParseMultipartForm(min(maxFileBytes, multipartFormMemory)); err != nil {
		var maxErr *http.MaxBytesError
		if errx.As(err, &maxErr) {
			return "", nil, ErrResourceExhausted.
				WithMessage("upload request is too large").
				WithHTTPStatus(http.StatusRequestEntityTooLarge)
		}
		return "", nil, ErrInvalidParam.WithMessage("")
	}
	if request.MultipartForm != nil {
		defer func() { _ = request.MultipartForm.RemoveAll() }()
	}
	file, header, err := request.FormFile(name)
	if err != nil {
		return "", nil, ErrInvalidParam.WithMessage("")
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return "", nil, errx.Wrap(err, "read multipart file")
	}
	if int64(len(data)) > maxFileBytes {
		return "", nil, ErrResourceExhausted.
			WithMessage("file is too large").
			WithHTTPStatus(http.StatusRequestEntityTooLarge)
	}
	return header.Filename, data, nil
}

// SetHeader 设置响应头。
func (c *Context) SetHeader(name, value string) {
	if response := c.Response(); response != nil {
		response.Header().Set(name, value)
	}
}

// JSON 以 JSON 格式写入响应。
func (c *Context) JSON(statusCode int, value any) error {
	response, err := c.responseWriter()
	if err != nil {
		return err
	}
	response.Header().Set("Content-Type", "application/json")
	c.writeHeader(statusCode)
	return json.NewEncoder(response).Encode(value)
}

// Text 以纯文本格式写入响应。
func (c *Context) Text(statusCode int, value string) error {
	response, err := c.responseWriter()
	if err != nil {
		return err
	}
	response.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.writeHeader(statusCode)
	_, err = io.WriteString(response, value)
	return err
}

// Blob 以指定 Content-Type 写入二进制响应。
func (c *Context) Blob(statusCode int, contentType string, data []byte) error {
	response, err := c.responseWriter()
	if err != nil {
		return err
	}
	if strings.TrimSpace(contentType) != "" {
		response.Header().Set("Content-Type", contentType)
	}
	c.writeHeader(statusCode)
	_, err = response.Write(data)
	return err
}

// NoContent 写入不包含响应体的状态码。
func (c *Context) NoContent(statusCode int) error {
	if _, err := c.responseWriter(); err != nil {
		return err
	}
	c.writeHeader(statusCode)
	return nil
}

func (c *Context) responseWriter() (http.ResponseWriter, error) {
	response := c.Response()
	if response == nil {
		return nil, errx.New("http response is unavailable")
	}
	return response, nil
}

func (c *Context) writeHeader(statusCode int) {
	c.http.committed = true
	c.http.response.WriteHeader(statusCode)
}

func (c *Context) responseCommitted() bool {
	if c == nil || c.http == nil {
		return false
	}
	if c.http.committed {
		return true
	}
	writer, ok := c.http.response.(*statusResponseWriter)
	return ok && writer.status != 0
}
