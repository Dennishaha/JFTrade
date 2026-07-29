package adk

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestSafeHTTPClientValidatesResolvedAddressAtDialTime(t *testing.T) {
	dialed := false
	client := newSafeHTTPClientWithNetwork(
		time.Second,
		func(_ context.Context, host string) error {
			if host == "10.0.0.8" {
				return errors.New("private rebound address blocked")
			}
			return nil
		},
		func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("10.0.0.8")}, nil
		},
		func(context.Context, string, string) (net.Conn, error) {
			dialed = true
			return nil, errors.New("must not dial")
		},
	)
	transport := client.Transport.(*http.Transport)
	if _, err := transport.DialContext(t.Context(), "tcp", "rebind.example:443"); err == nil || !strings.Contains(err.Error(), "rebound") {
		t.Fatalf("rebound address error = %v", err)
	}
	if dialed {
		t.Fatal("unsafe rebound address reached the network dialer")
	}
}

func TestSafeHTTPClientDialBoundaries(t *testing.T) {
	lookupErr := errors.New("lookup failed")
	dialErr := errors.New("dial failed")
	tests := []struct {
		name    string
		address string
		lookup  safeHostLookup
		dial    safeHostDial
		want    string
	}{
		{
			name: "malformed address", address: "missing-port",
			lookup: func(context.Context, string, string) ([]netip.Addr, error) { return nil, nil },
			dial:   func(context.Context, string, string) (net.Conn, error) { return nil, nil },
			want:   "missing port",
		},
		{
			name: "lookup failure", address: "example.test:443",
			lookup: func(context.Context, string, string) ([]netip.Addr, error) { return nil, lookupErr },
			dial:   func(context.Context, string, string) (net.Conn, error) { return nil, nil },
			want:   "resolve host",
		},
		{
			name: "dial failure", address: "203.0.113.1:443",
			lookup: func(context.Context, string, string) ([]netip.Addr, error) { return nil, nil },
			dial:   func(context.Context, string, string) (net.Conn, error) { return nil, dialErr },
			want:   "dial failed",
		},
		{
			name: "no addresses", address: "empty.test:443",
			lookup: func(context.Context, string, string) ([]netip.Addr, error) { return nil, nil },
			dial:   func(context.Context, string, string) (net.Conn, error) { return nil, nil },
			want:   "no usable addresses",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newSafeHTTPClientWithNetwork(time.Second, func(context.Context, string) error { return nil }, test.lookup, test.dial)
			transport := client.Transport.(*http.Transport)
			if _, err := transport.DialContext(t.Context(), "tcp", test.address); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DialContext() error = %v, want %q", err, test.want)
			}
		})
	}
}
