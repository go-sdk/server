package standard

import (
	"context"
	"strings"

	"github.com/go-sdk/core/errx"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
)

func unaryJWTAuthInterceptor(secret []byte) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if len(secret) == 0 || shouldSkipMethodAuth(info.FullMethod) {
			return handler(ctx, req)
		}
		ctx, err := contextWithJWT(ctx, firstMetadataValue(ctx, "authorization"), secret)
		if err != nil {
			return nil, ErrUnauthenticated.WithMessage("invalid bearer token")
		}
		return handler(ctx, req)
	}
}

func streamJWTAuthInterceptor(secret []byte) grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if len(secret) == 0 || shouldSkipMethodAuth(info.FullMethod) {
			return handler(srv, stream)
		}
		ctx, err := contextWithJWT(stream.Context(), firstMetadataValue(stream.Context(), "authorization"), secret)
		if err != nil {
			return ErrUnauthenticated.WithMessage("invalid bearer token")
		}
		return handler(srv, &contextServerStream{ServerStream: stream, ctx: ctx})
	}
}

func contextWithJWT(ctx context.Context, authorization string, secret []byte) (context.Context, error) {
	claims, err := parseJWTClaims(authorization, secret)
	if err != nil {
		return nil, err
	}
	ctx = contextWithAuthorization(ctx, authorization)
	return NewContext(ctx, JWTKey, claims), nil
}

func parseJWTClaims(authorization string, secret []byte) (jwt.MapClaims, error) {
	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, errx.New("missing bearer token")
	}
	token, err := jwt.Parse(parts[1], func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errx.New("unexpected jwt signing method")
		}
		return secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, errx.Wrap(err, "parse jwt")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errx.New("invalid jwt claims")
	}
	return claims, nil
}
