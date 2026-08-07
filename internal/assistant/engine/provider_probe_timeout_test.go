package adk

import (
	"testing"
	"time"
)

func TestProviderProbeTimeoutCapsConfiguredRequestTimeout(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		want     time.Duration
	}{
		{name: "default request timeout", provider: Provider{}, want: maxProviderProbeTimeout},
		{name: "short configured timeout", provider: Provider{RequestTimeoutMs: 15_000}, want: 15 * time.Second},
		{name: "long configured timeout", provider: Provider{RequestTimeoutMs: 600_000}, want: maxProviderProbeTimeout},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := providerProbeTimeout(test.provider); got != test.want {
				t.Fatalf("providerProbeTimeout() = %s, want %s", got, test.want)
			}
		})
	}
}
