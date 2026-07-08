package origin

import (
	"net/http"
	"net/url"
	"strings"
)

// Canonical normalizes request origins to scheme://host for CORS and auth checks.
func Canonical(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "http", "https", "wails":
	default:
		return ""
	}
	return scheme + "://" + strings.ToLower(parsed.Host)
}

// FromRequest returns the normalized Origin header, falling back to Referer.
func FromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if origin := Canonical(r.Header.Get("Origin")); origin != "" {
		return origin
	}
	return Canonical(r.Header.Get("Referer"))
}
