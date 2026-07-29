package adk

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestProviderHTTPClientAllowsPrivateNetworkProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	response, err := newProviderHTTPClient(time.Second).Do(req)
	if err != nil {
		t.Fatalf("private provider request failed: %v", err)
	}
	defer func() { jftradeCheckTestError(t, response.Body.Close()) }()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}
}

func TestValidateProviderBaseURLRejectsMetadataTargets(t *testing.T) {
	for _, rawURL := range []string{
		"http://169.254.169.254/latest/meta-data",
		"http://100.100.100.200/latest/meta-data",
		"http://metadata.google.internal/computeMetadata/v1",
		"http://[fd00:ec2::254]/latest/meta-data",
	} {
		if err := validateProviderBaseURL(rawURL); err == nil {
			t.Errorf("validateProviderBaseURL(%q) succeeded, want metadata rejection", rawURL)
		}
	}
}

func TestProviderHTTPClientRevalidatesRedirectDNSResolution(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		parsed, err := url.Parse(serverURLWithHost(t, "rebound.test", serverAddressPort(t, server)))
		if err != nil {
			t.Fatalf("parse redirect URL: %v", err)
		}
		parsed.Path = "/redirected"
		w.Header().Set("Location", parsed.String())
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	lookup := func(_ context.Context, _ string, host string) ([]netip.Addr, error) {
		switch strings.TrimSuffix(host, ".") {
		case "model.test":
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		case "rebound.test":
			return []netip.Addr{netip.MustParseAddr("169.254.169.254")}, nil
		default:
			return nil, &net.DNSError{Name: host, IsNotFound: true}
		}
	}

	client := newProviderHTTPClientWithResolver(time.Second, lookup)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, serverURLWithHost(t, "model.test", serverAddressPort(t, server)), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := client.Do(req)
	if resp != nil {
		defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	}
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("redirect request error = %v, want blocked metadata resolution", err)
	}
}

func serverAddressPort(t *testing.T, server *httptest.Server) string {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	return parsed.Port()
}

func serverURLWithHost(t *testing.T, host string, port string) string {
	t.Helper()
	if port == "" {
		t.Fatal("test server port is empty")
	}
	return "http://" + net.JoinHostPort(host, port)
}
