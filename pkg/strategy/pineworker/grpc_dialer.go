package pineworker

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const grpcUnlimitedMessageBytes = 1<<31 - 1

type GRPCDialerConfig struct {
	MaxMessageBytes int
	DialOptions     []grpc.DialOption
}

type GRPCDialer struct {
	config GRPCDialerConfig
}

func NewGRPCDialer(config GRPCDialerConfig) *GRPCDialer {
	if config.MaxMessageBytes <= 0 {
		config.MaxMessageBytes = grpcUnlimitedMessageBytes
	}
	return &GRPCDialer{config: config}
}

func (dialer *GRPCDialer) Dial(ctx context.Context, address string) (ManagedTransport, error) {
	if dialer == nil {
		return nil, fmt.Errorf("pine worker grpc dialer is nil")
	}
	options := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(dialer.config.MaxMessageBytes),
			grpc.MaxCallSendMsgSize(dialer.config.MaxMessageBytes),
		),
	}
	options = append(options, dialer.config.DialOptions...)
	conn, err := grpc.NewClient(address, options...)
	if err != nil {
		return nil, err
	}
	return &managedGRPCTransport{
		GRPCTransport: NewGRPCTransport(conn),
		conn:          conn,
	}, nil
}

type managedGRPCTransport struct {
	*GRPCTransport
	conn *grpc.ClientConn
}

func (transport *managedGRPCTransport) Close() error {
	if transport == nil || transport.conn == nil {
		return nil
	}
	return transport.conn.Close()
}
