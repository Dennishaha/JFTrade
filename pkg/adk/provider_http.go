package adk

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

var blockedProviderHostnames = map[string]struct{}{
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

func newProviderHTTPClient(timeout time.Duration) *http.Client {
	return newProviderHTTPClientWithResolver(timeout, net.DefaultResolver.LookupNetIP)
}

func newProviderHTTPClientWithResolver(
	timeout time.Duration,
	lookup func(context.Context, string, string) ([]netip.Addr, error),
) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := jftradeCheckedTypeAssertion[*http.Transport](http.DefaultTransport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if err := validateProviderHostname(host); err != nil {
			return nil, err
		}
		resolved, err := lookup(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, ip := range resolved {
			if err := validateProviderIP(ip); err != nil {
				lastErr = err
				continue
			}
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("provider host %q resolved to no usable addresses", host)
		}
		return nil, lastErr
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many provider redirects")
			}
			return validateProviderHostname(req.URL.Hostname())
		},
	}
}

func validateProviderHostname(host string) error {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" {
		return fmt.Errorf("provider host is required")
	}
	if _, blocked := blockedProviderHostnames[host]; blocked {
		return fmt.Errorf("provider metadata host %q is blocked", host)
	}
	if ip, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return validateProviderIP(ip)
	}
	return nil
}

func validateProviderIP(ip netip.Addr) error {
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

func jftradeCheckedTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		panic("unexpected dynamic type")
	}
	return typed
}

func jftradeOptionalTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		var zero T
		return zero
	}
	return typed
}
