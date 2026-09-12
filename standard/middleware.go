package standard

import (
	"context"

	"buf.build/go/protovalidate"
	"google.golang.org/grpc"
)

type contextServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *contextServerStream) Context() context.Context { return s.ctx }

func newValidator() (protovalidate.Validator, error) {
	return protovalidate.New()
}
