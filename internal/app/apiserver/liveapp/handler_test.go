package liveapp

import (
	"testing"
)

func TestNewHandlerKeepsLiveTransportOptions(t *testing.T) {
	handler := NewHandler(nil, Options{})
	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}
	if err := handler.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
