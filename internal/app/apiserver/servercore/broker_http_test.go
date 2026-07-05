package servercore

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"testing"
)

func decodeBrokerEnvelope(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := jftradeTestHTTPGet(t, url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d", url, resp.StatusCode)
	}
	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true for %s", url)
	}
	return envelope.Data
}

func portFromAddr(t *testing.T, addr string) int {
	t.Helper()
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	value, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("Atoi(%q): %v", port, err)
	}
	return value
}
