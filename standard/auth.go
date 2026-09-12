package standard

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-sdk/core/errx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func unaryJWTAuthInterceptor(secret []byte) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if len(secret) == 0 || shouldSkipMethodAuth(info.FullMethod) {
			return handler(ctx, req)
		}
		ctx, err := contextWithJWT(ctx, firstMetadataValue(ctx, "authorization"), secret)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid bearer token")
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
			return status.Error(codes.Unauthenticated, "invalid bearer token")
		}
		return handler(srv, &contextServerStream{ServerStream: stream, ctx: ctx})
	}
}

func jwtHTTPHandler(secret []byte, handler runtime.HandlerFunc) runtime.HandlerFunc {
	if len(secret) == 0 {
		return handler
	}
	return func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
		ctx, err := contextWithJWT(r.Context(), r.Header.Get("Authorization"), secret)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		handler(w, r.WithContext(ctx), pathParams)
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
