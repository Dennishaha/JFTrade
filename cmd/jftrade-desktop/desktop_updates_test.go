package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDesktopUpdateServiceSelectsLatestStableDesktopRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "JFTrade-Desktop/1.2.0" {
			t.Fatalf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		_, _ = w.Write([]byte(`[
			{"tag_name":"v9.0.0","html_url":"https://example.invalid/general","draft":false,"prerelease":false},
			{"tag_name":"desktop-v1.3.0-beta.1","html_url":"https://example.invalid/beta","draft":false,"prerelease":true},
			{"tag_name":"desktop-v1.4.0","html_url":"https://example.invalid/1.4.0","body":"new","published_at":"2026-07-10T00:00:00Z","draft":false,"prerelease":false},
			{"tag_name":"desktop-v1.3.0","html_url":"https://example.invalid/1.3.0","draft":false,"prerelease":false}
		]`))
	}))
	defer server.Close()

	service := &DesktopUpdateService{enabled: true, current: "1.2.0", releasesURL: server.URL, client: server.Client()}
	result, err := service.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.Available || result.LatestVersion != "1.4.0" || result.ReleaseURL != "https://example.invalid/1.4.0" {
		t.Fatalf("result = %#v", result)
	}
}

func TestDesktopUpdateServiceDisabledForDevelopment(t *testing.T) {
	result, err := (&DesktopUpdateService{enabled: false, current: "dev"}).Check()
	if err != nil || result.Available || result.CurrentVersion != "dev" {
		t.Fatalf("development update result = %#v, err=%v", result, err)
	}
}
