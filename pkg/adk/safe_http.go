package adk

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"
)

type safeHostValidator func(context.Context, string) error
type safeHostLookup func(context.Context, string, string) ([]netip.Addr, error)
type safeHostDial func(context.Context, string, string) (net.Conn, error)

func newSafeHTTPClient(timeout time.Duration, validate safeHostValidator) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return newSafeHTTPClientWithNetwork(timeout, validate, net.DefaultResolver.LookupNetIP, dialer.DialContext)
}

func newSafeHTTPClientWithNetwork(timeout time.Duration, validate safeHostValidator, lookup safeHostLookup, dial safeHostDial) *http.Client {
	client := &http.Client{Timeout: timeout}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		// Tests and embedders may intentionally provide a complete transport.
		client.Transport = http.DefaultTransport
		return client
	}
	transport := base.Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses := []netip.Addr(nil)
		if literal, parseErr := netip.ParseAddr(host); parseErr == nil {
			addresses = append(addresses, literal)
		} else {
			addresses, err = lookup(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("resolve host for connection: %w", err)
			}
		}
		var lastErr error
		for _, resolved := range addresses {
			if err := validate(ctx, resolved.Unmap().String()); err != nil {
				lastErr = err
				continue
			}
			conn, dialErr := dial(ctx, network, net.JoinHostPort(resolved.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("host %q resolved to no usable addresses", host)
		}
		return nil, lastErr
	}
	client.Transport = transport
	return client
}
