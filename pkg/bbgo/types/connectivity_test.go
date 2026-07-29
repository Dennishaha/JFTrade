package types

import (
	"testing"
	"time"
)

func TestConnectivity(t *testing.T) {
	t.Run("general", func(t *testing.T) {
		conn1 := NewConnectivity()
		assertConnectivityState(t, conn1, false, false)
		conn1.setConnect()
		assertConnectivityState(t, conn1, true, false)
		conn1.setAuthed()
		assertConnectivityState(t, conn1, true, true)
		conn1.setDisconnect()
		assertConnectivityState(t, conn1, false, false)
	})

	t.Run("reconnect", func(t *testing.T) {
		conn1 := NewConnectivity()
		for range 2 {
			conn1.setConnect()
			assertConnectivityState(t, conn1, true, false)
			conn1.setAuthed()
			assertConnectivityState(t, conn1, true, true)
			conn1.setDisconnect()
			assertConnectivityState(t, conn1, false, false)
		}
	})

	t.Run("no-auth reconnect", func(t *testing.T) {
		conn1 := NewConnectivity()
		for range 2 {
			conn1.setConnect()
			assertConnectivityState(t, conn1, true, false)
			conn1.setDisconnect()
			assertConnectivityState(t, conn1, false, false)
		}
	})
}

func assertConnectivityState(t *testing.T, connectivity *Connectivity, connected, authed bool) {
	t.Helper()
	if connectivity.IsConnected() != connected || connectivity.IsAuthed() != authed {
		t.Fatalf(
			"connectivity state = connected:%v authed:%v, want connected:%v authed:%v",
			connectivity.IsConnected(), connectivity.IsAuthed(), connected, authed,
		)
	}
	if channelClosed(connectivity.ConnectedC()) != connected {
		t.Fatalf("connected channel closed = %v, want %v", channelClosed(connectivity.ConnectedC()), connected)
	}
	if channelClosed(connectivity.AuthedC()) != authed {
		t.Fatalf("authed channel closed = %v, want %v", channelClosed(connectivity.AuthedC()), authed)
	}
	if channelClosed(connectivity.DisconnectedC()) == connected {
		t.Fatalf("disconnected channel closed = %v, want %v", channelClosed(connectivity.DisconnectedC()), !connected)
	}
}

func channelClosed(channel <-chan struct{}) bool {
	select {
	case <-channel:
		return true
	default:
		return false
	}
}

func waitSigChan(c <-chan struct{}, timeoutDuration time.Duration) bool {
	select {
	case <-time.After(timeoutDuration):
		return false

	case <-c:
		return true
	}
}
