package standard

import (
	"net/http"
	"runtime/debug"

	"github.com/go-sdk/core/errx"
	"github.com/go-sdk/core/logx"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) httpRouteHandler(handler HandlerFunc) runtime.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request, pathParams map[string]string) {
		ctx := NewHTTPContext(request, response, pathParams)
		defer func() {
			if recovered := recover(); recovered != nil {
				logx.Ctx(ctx).Error().
					Interface("panic", recovered).
					Bytes("stack", debug.Stack()).
					Msg("http route handler panic recovered")
				s.renderHTTPError(ctx, ErrInternal)
			}
		}()

		if len(s.config.jwtSecret) > 0 {
			requestContext, err := contextWithJWT(
				request.Context(),
				request.Header.Get("Authorization"),
				s.config.jwtSecret,
			)
			if err != nil {
				s.renderHTTPError(ctx, ErrUnauthenticated.WithMessage("invalid bearer token"))
				return
			}
			request = request.WithContext(requestContext)
			ctx = NewHTTPContext(request, response, pathParams)
		}

		if err := handler(ctx); err != nil {
			s.renderHTTPError(ctx, err)
		}
	}
}

func (s *Server) renderHTTPError(ctx *Context, handlerErr error) {
	if ctx.responseCommitted() {
		logx.Ctx(ctx).Error().Err(handlerErr).Msg("http route handler returned error after response committed")
		return
	}

	converted := convertResponseError(handlerErr, s.config.errorConverters)
	httpStatus := 0
	var responseError RespError
	if errx.As(converted, &responseError) {
		httpStatus = responseError.httpStatus
	}
	grpcStatus, ok := status.FromError(converted)
	if !ok || grpcStatus.Code() == codes.OK {
		logx.Ctx(ctx).Error().Err(converted).Msg("http route handler returned internal error")
		grpcStatus = ErrInternal.GRPCStatus()
	}
	response, err := newGatewayErrorResponse(grpcStatus.Proto())
	if err != nil {
		logx.Ctx(ctx).Error().Err(err).Msg("build http route error response")
		grpcStatus = ErrInternal.GRPCStatus()
		httpStatus = 0
		response, _ = newGatewayErrorResponse(grpcStatus.Proto())
	}
	if httpStatus < 400 || httpStatus > 599 {
		httpStatus = runtime.HTTPStatusFromCode(grpcStatus.Code())
	}
	if err = ctx.JSON(httpStatus, response); err != nil {
		logx.Ctx(ctx).Error().Err(err).Msg("write http route error response")
	}
}
