// Package providers contains external model-provider transport adapters.
package providers

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"time"
)

var blockedHostnames = map[string]struct{}{
	"metadata":                   {},
	"metadata.google.internal":   {},
	"instance-data":              {},
	"instance-data.ec2.internal": {},
}

var blockedMetadataPrefixes = []netip.Prefix{
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("100.100.100.200/32"),
	netip.MustParsePrefix("fd00:ec2::254/128"),
}

// NewHTTPClient builds a provider client that rejects metadata and link-local endpoints.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return NewHTTPClientWithResolver(timeout, net.DefaultResolver.LookupNetIP)
}

// NewHTTPClientWithResolver allows deterministic DNS behavior in tests.
func NewHTTPClientWithResolver(
	timeout time.Duration,
	lookup func(context.Context, string, string) ([]netip.Addr, error),
) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		panic("unexpected HTTP default transport type")
	}
	checkedTransport := transport.Clone()
	checkedTransport.Proxy = nil
	checkedTransport.DialContext = func(
		ctx context.Context,
		network string,
		address string,
	) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if err := ValidateHostname(host); err != nil {
			return nil, err
		}
		resolved, err := lookup(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, ip := range resolved {
			if err := ValidateIP(ip); err != nil {
				lastErr = err
				continue
			}
			conn, dialErr := dialer.DialContext(
				ctx,
				network,
				net.JoinHostPort(ip.String(), port),
			)
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr == nil {
			lastErr = fmt.Errorf(
				"provider host %q resolved to no usable addresses",
				host,
			)
		}
		return nil, lastErr
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: checkedTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many provider redirects")
			}
			return ValidateHostname(req.URL.Hostname())
		},
	}
}

// ValidateHostname rejects provider endpoints that target metadata services.
func ValidateHostname(host string) error {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" {
		return fmt.Errorf("provider host is required")
	}
	if _, blocked := blockedHostnames[host]; blocked {
		return fmt.Errorf("provider metadata host %q is blocked", host)
	}
	if ip, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return ValidateIP(ip)
	}
	return nil
}

// ValidateIP rejects unsafe provider destinations while allowing loopback test servers.
func ValidateIP(ip netip.Addr) error {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsUnspecified() {
		return fmt.Errorf("provider address %q is unspecified", ip)
	}
	if ip.IsMulticast() {
		return fmt.Errorf("provider multicast address %q is blocked", ip)
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("provider link-local address %q is blocked", ip)
	}
	for _, prefix := range blockedMetadataPrefixes {
		if prefix.Contains(ip) {
			return fmt.Errorf("provider metadata address %q is blocked", ip)
		}
	}
	return nil
}

// RejectUnsafeHost rejects local, private, link-local, multicast, unspecified,
// and metadata endpoints for user-supplied HTTP targets such as skill installs
// and http.fetch.
func RejectUnsafeHost(ctx context.Context, host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("host is required")
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return fmt.Errorf("localhost targets are blocked")
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if unsafeAddr(addr) {
			return fmt.Errorf("private, loopback, link-local, multicast and metadata addresses are blocked")
		}
		return nil
	}
	resolver := net.DefaultResolver
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("resolve host: %w", err)
	}
	if slices.ContainsFunc(addrs, unsafeAddr) {
		return fmt.Errorf("private, loopback, link-local, multicast and metadata addresses are blocked")
	}
	return nil
}

func unsafeAddr(addr netip.Addr) bool {
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return true
	}
	if addr.String() == "169.254.169.254" {
		return true
	}
	return false
}

// IsUnsafeAddr reports whether an IP must not be dialed for user-supplied
// HTTP targets.
func IsUnsafeAddr(addr netip.Addr) bool {
	return unsafeAddr(addr)
}
