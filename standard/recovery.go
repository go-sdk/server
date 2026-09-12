package standard

import (
	"context"
	"net/http"
	"runtime/debug"

	"github.com/go-sdk/core/logx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func httpRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logx.Ctx(r.Context()).Error().
					Interface("panic", recovered).
					Bytes("stack", debug.Stack()).
					Msg("http handler panic recovered")
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func recoveryHandler(ctx context.Context, recovered any) error {
	logx.Ctx(ctx).Error().
		Interface("panic", recovered).
		Bytes("stack", debug.Stack()).
		Msg("grpc handler panic recovered")
	return status.Error(codes.Internal, "internal server error")
}
