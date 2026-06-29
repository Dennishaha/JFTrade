package pineworker

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker/pineworkerpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestGRPCTransportRunScriptAndHealthCheck(t *testing.T) {
	server := grpc.NewServer()
	pineworkerpb.RegisterPineWorkerServer(server, testPineWorkerServer{})
	listener := bufconn.Listen(1024 * 1024)
	go func() {
		if err := server.Serve(listener); err != nil {
			t.Errorf("serve bufconn: %v", err)
		}
	}()
	t.Cleanup(func() {
		server.Stop()
		if err := listener.Close(); err != nil {
			t.Errorf("close listener: %v", err)
		}
	})

	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return listener.DialContext(ctx)
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close conn: %v", err)
		}
	})

	transport := NewGRPCTransport(conn)
	health, err := transport.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck error = %v", err)
	}
	if !health.OK || health.WorkerID != "worker-1" || health.PineTSVersion != "pinets-test" {
		t.Fatalf("unexpected health: %#v", health)
	}

	response, err := transport.RunScript(context.Background(), validClientRequest())
	if err != nil {
		t.Fatalf("RunScript error = %v", err)
	}
	if response.JobID != "job-1" || response.Plots[0].Name != "signal" {
		t.Fatalf("unexpected response: %#v", response)
	}
	if response.OrderIntents[0].ID != "long" || !response.OrderIntents[0].HasQuantity {
		t.Fatalf("unexpected order intent: %#v", response.OrderIntents)
	}
	if response.Metadata.Duration != 15*time.Millisecond || response.Metadata.WorkerID != "worker-1" {
		t.Fatalf("unexpected metadata: %#v", response.Metadata)
	}
}

func TestGRPCTransportRequiresClient(t *testing.T) {
	_, err := NewGRPCTransportWithClient(nil)
	if err == nil {
		t.Fatal("NewGRPCTransportWithClient error = nil, want error")
	}
	var transport *GRPCTransport
	if _, err := transport.RunScript(context.Background(), validClientRequest()); err == nil {
		t.Fatal("nil transport RunScript error = nil, want error")
	}
}

type testPineWorkerServer struct {
	pineworkerpb.UnimplementedPineWorkerServer
}

func (testPineWorkerServer) HealthCheck(context.Context, *pineworkerpb.HealthCheckRequest) (*pineworkerpb.HealthCheckResponse, error) {
	return &pineworkerpb.HealthCheckResponse{
		Ok:            true,
		WorkerId:      "worker-1",
		Version:       "0.1.0",
		PinetsVersion: "pinets-test",
		Capabilities:  []string{"run"},
	}, nil
}

func (testPineWorkerServer) RunScript(ctx context.Context, request *pineworkerpb.RunScriptRequest) (*pineworkerpb.RunScriptResponse, error) {
	return &pineworkerpb.RunScriptResponse{
		JobId: request.GetJobId(),
		Plots: []*pineworkerpb.PlotOutput{{
			Name:   "signal",
			Values: []float64{1},
		}},
		OrderIntents: []*pineworkerpb.OrderIntent{{
			Kind:        "entry",
			Id:          "long",
			Direction:   "long",
			Quantity:    1,
			BarIndex:    0,
			Time:        request.GetCandles()[0].GetOpenTime(),
			HasQuantity: true,
		}},
		Metadata: &pineworkerpb.WorkerMetadata{
			WorkerId:      "worker-1",
			Version:       "0.1.0",
			PinetsVersion: "pinets-test",
			DurationMs:    15,
			RequestBytes:  100,
			ResponseBytes: 100,
		},
	}, nil
}
