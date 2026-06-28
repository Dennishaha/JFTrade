package pineworker

import (
	"context"
	"fmt"

	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker/pineworkerpb"
	"google.golang.org/grpc"
)

type HealthStatus struct {
	OK            bool
	WorkerID      string
	Version       string
	PineTSVersion string
	Capabilities  []string
}

type GRPCTransport struct {
	client pineworkerpb.PineWorkerClient
}

func NewGRPCTransport(conn grpc.ClientConnInterface) *GRPCTransport {
	return &GRPCTransport{client: pineworkerpb.NewPineWorkerClient(conn)}
}

func NewGRPCTransportWithClient(client pineworkerpb.PineWorkerClient) (*GRPCTransport, error) {
	if client == nil {
		return nil, fmt.Errorf("pine worker grpc client is required")
	}
	return &GRPCTransport{client: client}, nil
}

func (transport *GRPCTransport) RunScript(ctx context.Context, request RunScriptRequest) (RunScriptResponse, error) {
	if transport == nil || transport.client == nil {
		return RunScriptResponse{}, fmt.Errorf("pine worker grpc transport is not initialized")
	}
	response, err := transport.client.RunScript(ctx, requestToProto(request))
	if err != nil {
		return RunScriptResponse{}, err
	}
	return responseFromProto(response), nil
}

func (transport *GRPCTransport) HealthCheck(ctx context.Context) (HealthStatus, error) {
	if transport == nil || transport.client == nil {
		return HealthStatus{}, fmt.Errorf("pine worker grpc transport is not initialized")
	}
	response, err := transport.client.HealthCheck(ctx, &pineworkerpb.HealthCheckRequest{})
	if err != nil {
		return HealthStatus{}, err
	}
	return healthFromProto(response), nil
}
