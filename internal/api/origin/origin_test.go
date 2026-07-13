package origin

import (
	"net/http"
	"testing"
)

func TestCanonical(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "  ", want: ""},
		{name: "http", value: " HTTP://Example.COM/path?q=1 ", want: "http://example.com"},
		{name: "https port", value: "https://Example.COM:8443/a", want: "https://example.com:8443"},
		{name: "wails", value: "wails://wails", want: "wails://wails"},
		{name: "missing host", value: "/relative/path", want: ""},
		{name: "unsupported scheme", value: "ftp://example.com", want: ""},
		{name: "invalid URL", value: "://bad", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Canonical(test.value); got != test.want {
				t.Fatalf("Canonical(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestFromRequest(t *testing.T) {
	if got := FromRequest(nil); got != "" {
		t.Fatalf("FromRequest(nil) = %q", got)
	}
	request, err := http.NewRequest(http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	request.Header.Set("Origin", "https://EXAMPLE.com/path")
	request.Header.Set("Referer", "https://fallback.example/path")
	if got := FromRequest(request); got != "https://example.com" {
		t.Fatalf("FromRequest origin = %q", got)
	}
	request.Header.Set("Origin", "invalid")
	if got := FromRequest(request); got != "https://fallback.example" {
		t.Fatalf("FromRequest referer = %q", got)
	}
}
