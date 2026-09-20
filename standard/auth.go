package standard

import (
	"context"
	"strings"

	"github.com/go-sdk/core/errx"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"

	"github.com/go-sdk/server/jwtx"
)

func unaryJWTAuthInterceptor(auth *jwtx.Parser) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if auth == nil || shouldSkipMethodAuth(info.FullMethod) {
			return handler(ctx, req)
		}
		ctx, err := contextWithJWT(ctx, firstMetadataValue(ctx, "authorization"), *auth)
		if err != nil {
			return nil, ErrUnauthenticated.WithMessage("invalid bearer token")
		}
		return handler(ctx, req)
	}
}

func streamJWTAuthInterceptor(auth *jwtx.Parser) grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if auth == nil || shouldSkipMethodAuth(info.FullMethod) {
			return handler(srv, stream)
		}
		ctx, err := contextWithJWT(stream.Context(), firstMetadataValue(stream.Context(), "authorization"), *auth)
		if err != nil {
			return ErrUnauthenticated.WithMessage("invalid bearer token")
		}
		return handler(srv, &contextServerStream{ServerStream: stream, ctx: ctx})
	}
}

func contextWithJWT(ctx context.Context, authorization string, parser jwtx.Parser) (context.Context, error) {
	claims, err := parseJWTClaims(authorization, parser)
	if err != nil {
		return nil, err
	}
	ctx = contextWithAuthorization(ctx, authorization)
	return NewContext(ctx, JWTKey, claims), nil
}

func parseJWTClaims(authorization string, parser jwtx.Parser) (jwt.MapClaims, error) {
	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, errx.New("missing bearer token")
	}
	claims := jwt.MapClaims{}
	if err := parser.Parse(parts[1], claims); err != nil {
		return nil, err
	}
	return claims, nil
}
