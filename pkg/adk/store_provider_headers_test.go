package adk

import (
	"context"
	"strings"
	"testing"
)

func TestSaveProviderValidatesBaseURLAndDefaultHeaders(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	for _, tc := range []struct {
		name string
		req  ProviderWriteRequest
		want string
	}{
		{
			name: "invalid scheme",
			req: ProviderWriteRequest{
				ID: "provider-ftp", DisplayName: "FTP Provider", BaseURL: "ftp://model.example.com/v1", Enabled: true,
			},
			want: "provider base URL must use http or https scheme",
		},
		{
			name: "missing host",
			req: ProviderWriteRequest{
				ID: "provider-no-host", DisplayName: "No Host", BaseURL: "https:///v1", Enabled: true,
			},
			want: "provider base URL must have a host",
		},
		{
			name: "forbidden host header",
			req: ProviderWriteRequest{
				ID: "provider-host-header", DisplayName: "Bad Header", BaseURL: "https://model.example.com/v1", Enabled: true,
				DefaultHeaders: map[string]string{"Host": "evil.example.com"},
			},
			want: `provider default header "Host" is not allowed`,
		},
		{
			name: "forbidden sec header",
			req: ProviderWriteRequest{
				ID: "provider-sec-header", DisplayName: "Sec Header", BaseURL: "https://model.example.com/v1", Enabled: true,
				DefaultHeaders: map[string]string{"Sec-Fetch-Site": "cross-site"},
			},
			want: `provider default header "Sec-Fetch-Site" is not allowed`,
		},
		{
			name: "forbidden proxy header",
			req: ProviderWriteRequest{
				ID: "provider-proxy-header", DisplayName: "Proxy Header", BaseURL: "https://model.example.com/v1", Enabled: true,
				DefaultHeaders: map[string]string{"Proxy-Authorization": "Basic bad"},
			},
			want: `provider default header "Proxy-Authorization" is not allowed`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := runtime.Store().SaveProvider(ctx, tc.req); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("SaveProvider error = %v, want %q", err, tc.want)
			}
		})
	}

	provider, err := runtime.Store().SaveProvider(ctx, ProviderWriteRequest{
		ID:          "provider-valid-headers",
		DisplayName: "Valid Headers",
		BaseURL:     "https://model.example.com/v1",
		Model:       "gpt-4o-mini",
		Enabled:     true,
		DefaultHeaders: map[string]string{
			" X-Trace-ID ": " trace-1 ",
			" ":            "ignored",
			"X-Empty":      "   ",
		},
	})
	if err != nil {
		t.Fatalf("SaveProvider valid headers: %v", err)
	}
	if provider.DefaultHeaders["X-Trace-ID"] != "trace-1" {
		t.Fatalf("normalized headers = %#v, want X-Trace-ID=trace-1", provider.DefaultHeaders)
	}
	if len(provider.DefaultHeaders) != 1 {
		t.Fatalf("normalized headers len = %d, want 1; headers=%#v", len(provider.DefaultHeaders), provider.DefaultHeaders)
	}
}
