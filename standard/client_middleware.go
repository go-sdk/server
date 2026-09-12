package standard

import (
	"context"
	"io"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func unaryClientContextInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, conn *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoker(outgoingRequestContext(ctx), method, req, reply, conn, opts...)
	}
}

func streamClientContextInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, conn *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return streamer(outgoingRequestContext(ctx), desc, conn, method, opts...)
	}
}

func outgoingRequestContext(ctx context.Context) context.Context {
	requestContext := FromContext(ctx)
	metadataValues, _ := metadata.FromOutgoingContext(ctx)
	metadataValues = metadataValues.Copy()
	if requestContext.RequestID() != "" {
		metadataValues.Set(requestIDMetadataKey, requestContext.RequestID())
	}
	if requestContext.authorization != "" {
		metadataValues.Set("authorization", requestContext.authorization)
	}
	metadataValues.Set(depthMetadataKey, strconv.Itoa(requestContext.Depth()+1))
	return metadata.NewOutgoingContext(ctx, metadataValues)
}

func unaryClientPayloadLoggingInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, conn *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if isHealthMethod(method) {
			return invoker(ctx, method, req, reply, conn, opts...)
		}
		redactPayload := shouldSkipMethodLog(method)
		logGRPCRequest(ctx, "client", method, req, redactPayload)
		startedAt := time.Now()
		err := invoker(ctx, method, req, reply, conn, opts...)
		logGRPCResponse(ctx, "client", method, reply, err, time.Since(startedAt), redactPayload)
		return err
	}
}

func streamClientPayloadLoggingInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, conn *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		stream, err := streamer(ctx, desc, conn, method, opts...)
		if err != nil {
			return nil, err
		}
		if isHealthMethod(method) {
			return stream, nil
		}
		return &payloadLoggingClientStream{
			ClientStream:  stream,
			ctx:           ctx,
			method:        method,
			redactPayload: shouldSkipMethodLog(method),
		}, nil
	}
}

type payloadLoggingClientStream struct {
	grpc.ClientStream
	ctx           context.Context
	method        string
	redactPayload bool
}

func (s *payloadLoggingClientStream) SendMsg(message any) error {
	logGRPCRequest(s.ctx, "client", s.method, message, s.redactPayload)
	return s.ClientStream.SendMsg(message)
}

func (s *payloadLoggingClientStream) RecvMsg(message any) error {
	startedAt := time.Now()
	err := s.ClientStream.RecvMsg(message)
	if err != io.EOF {
		logGRPCResponse(s.ctx, "client", s.method, message, err, time.Since(startedAt), s.redactPayload)
	}
	return err
}
