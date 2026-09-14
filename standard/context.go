package standard

import (
	"context"
	"maps"

	"github.com/go-sdk/core/osx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/cast"
)

const (
	TraceIDKey        = "trace-id"
	SpanIDKey         = "span-id"
	JWTKey            = "jwt"
	DepthKey          = "x-depth"
	ClientIPKey       = "client-ip"
	ContentTypeKey    = "content-type"
	UserAgentKey      = "user-agent"
	AcceptLanguageKey = "accept-language"
)

type contextKey struct{}

// Context 提供标准中间件写入的请求级参数，以及额外 HTTP Handler 的请求和响应能力。
type Context struct {
	context.Context

	values        map[string]any
	authorization string
	http          *httpContext
}

// NewContext 在保留已有参数的基础上创建新的请求上下文。
func NewContext(ctx context.Context, kvs ...any) context.Context {
	if len(kvs)%2 != 0 {
		osx.Panic("standard context key-value pairs must have an even length")
	}
	ctx = contextOrBackground(ctx)
	current := FromContext(ctx)
	values := maps.Clone(current.values)
	for i := 0; i < len(kvs); i += 2 {
		values[cast.ToString(kvs[i])] = kvs[i+1]
	}
	return context.WithValue(ctx, contextKey{}, &Context{
		Context:       ctx,
		values:        values,
		authorization: current.authorization,
		http:          current.http,
	})
}

// FromContext 返回请求上下文中的扩展参数视图。
func FromContext(ctx context.Context) *Context {
	if ctx == nil {
		osx.Panic("context must not be nil")
	}
	if current, ok := ctx.Value(contextKey{}).(*Context); ok {
		return current
	}
	return &Context{Context: ctx, values: map[string]any{}}
}

func (c *Context) Get(key any) any {
	return c.values[cast.ToString(key)]
}

func (c *Context) Value(key any) any {
	if _, ok := key.(contextKey); ok {
		return c
	}
	return c.Context.Value(key)
}

func (c *Context) String(key any) string {
	return cast.ToString(c.Get(key))
}

func (c *Context) TraceID() string { return c.String(TraceIDKey) }

// SpanID 返回当前服务内单次请求到响应的处理标识。
func (c *Context) SpanID() string { return c.String(SpanIDKey) }

func (c *Context) Depth() int {
	depth := cast.ToInt(c.Get(DepthKey))
	if depth < 0 {
		return 0
	}
	return depth
}

func (c *Context) ClientIP() string { return c.String(ClientIPKey) }

func (c *Context) ContentType() string { return c.String(ContentTypeKey) }

func (c *Context) UserAgent() string { return c.String(UserAgentKey) }

// AcceptLanguage 返回请求携带的原始语言偏好。
func (c *Context) AcceptLanguage() string { return c.String(AcceptLanguageKey) }

// JWT 返回鉴权中间件验证后的 Claims 副本。
func (c *Context) JWT() jwt.MapClaims {
	claims, _ := c.Get(JWTKey).(jwt.MapClaims)
	return maps.Clone(claims)
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func contextWithAuthorization(ctx context.Context, authorization string) context.Context {
	current := FromContext(ctx)
	return context.WithValue(ctx, contextKey{}, &Context{
		Context:       ctx,
		values:        maps.Clone(current.values),
		authorization: authorization,
		http:          current.http,
	})
}
