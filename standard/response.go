package standard

import (
	"context"
	"encoding/json"

	"github.com/go-sdk/core/errx"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type gatewayErrorResponse struct {
	Code    int32             `json:"code"`
	Message string            `json:"message"`
	Domain  string            `json:"domain"`
	Reason  string            `json:"reason"`
	Details []json.RawMessage `json:"details"`
}

func gatewayResponseRewriter(_ context.Context, response proto.Message) (any, error) {
	statusResponse, ok := response.(*statuspb.Status)
	if !ok || statusResponse.GetCode() == 0 {
		return map[string]any{"data": response}, nil
	}
	return newGatewayErrorResponse(statusResponse)
}

func newGatewayErrorResponse(statusResponse *statuspb.Status) (gatewayErrorResponse, error) {
	result := gatewayErrorResponse{
		Code:    statusResponse.GetCode(),
		Message: statusResponse.GetMessage(),
		Details: make([]json.RawMessage, 0),
	}
	for _, detail := range statusResponse.GetDetails() {
		if detail.MessageIs(&errdetails.ErrorInfo{}) {
			info := &errdetails.ErrorInfo{}
			if err := detail.UnmarshalTo(info); err == nil {
				result.Domain = info.GetDomain()
				result.Reason = info.GetReason()
				continue
			}
		}
		encoded, err := protojson.Marshal(detail)
		if err != nil {
			return gatewayErrorResponse{}, errx.Wrap(err, "marshal grpc error detail")
		}
		result.Details = append(result.Details, encoded)
	}
	return result, nil
}
