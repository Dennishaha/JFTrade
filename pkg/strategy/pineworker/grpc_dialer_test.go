package pineworker

import (
	"context"
	"strings"
	"testing"
)

func TestGRPCDialerCreatesManagedTransport(t *testing.T) {
	dialer := NewGRPCDialer(GRPCDialerConfig{MaxMessageBytes: 1024})
	transport, err := dialer.Dial(context.Background(), "127.0.0.1:1")
	if err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	if transport == nil {
		t.Fatal("transport = nil")
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
}

func TestGRPCDialerRejectsNilReceiver(t *testing.T) {
	var dialer *GRPCDialer
	_, err := dialer.Dial(context.Background(), "127.0.0.1:1")
	if err == nil || !strings.Contains(err.Error(), "dialer is nil") {
		t.Fatalf("Dial error = %v, want nil dialer", err)
	}
}
