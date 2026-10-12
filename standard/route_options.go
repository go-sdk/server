package standard

// RouteOption 配置单个额外 HTTP 路由，不影响其他路由或 RPC。
type RouteOption func(*routeConfig) error

type routeConfig struct {
	skipAuth bool
}

// WithRouteSkipAuth 跳过当前路由的默认 JWT 校验，由 Handler 负责所需的业务鉴权。
func WithRouteSkipAuth() RouteOption {
	return func(c *routeConfig) error {
		c.skipAuth = true
		return nil
	}
}
