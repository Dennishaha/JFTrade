package adk

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"
)

func TestRejectUnsafeHostCoversDNSAndLocalhostBoundaries(t *testing.T) {
	if err := rejectUnsafeHost(context.Background(), "api.localhost"); err == nil || !strings.Contains(err.Error(), "localhost targets are blocked") {
		t.Fatalf("rejectUnsafeHost(.localhost) err = %v", err)
	}
	if err := rejectUnsafeHost(context.Background(), "::1"); err == nil || !strings.Contains(err.Error(), "private, loopback") {
		t.Fatalf("rejectUnsafeHost(::1) err = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rejectUnsafeHost(ctx, "example.invalid"); err == nil || !strings.Contains(err.Error(), "resolve host") {
		t.Fatalf("rejectUnsafeHost(cancelled dns) err = %v", err)
	}

	// Use a documentation domain that resolves publicly in normal environments.
	if err := rejectUnsafeHost(context.Background(), "example.com"); err != nil {
		if dnsErr := new(net.DNSError); !strings.Contains(err.Error(), "resolve host") || !strings.Contains(err.Error(), dnsErr.Err) {
			t.Fatalf("rejectUnsafeHost(example.com) err = %v", err)
		}
	}
}

func TestUnsafeAddrCoversIPv6MetadataAndDocumentationRanges(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"::", true},
		{"::1", true},
		{"fe80::1", true},
		{"ff02::1", true},
		{"fc00::1", true},
		{"2001:4860:4860::8888", false},
		{"198.51.100.10", false},
	}
	for _, tc := range cases {
		addr := netip.MustParseAddr(tc.addr)
		if got := unsafeAddr(addr); got != tc.want {
			t.Fatalf("unsafeAddr(%s) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}
