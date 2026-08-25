package rustrehearsal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	"github.com/jftrade/jftrade-main/internal/api/middleware"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/webaccess"
)

const (
	defaultRehearsalProxyTimeout = 30 * time.Second
	maxRehearsalResponseBytes    = 16 * 1024 * 1024
)

var rehearsalResponseHeaders = []string{
	"Cache-Control", "Content-Language", "Content-Type", "ETag",
	"Expires", "Last-Modified", "Retry-After", "Set-Cookie", "Vary",
	"X-Content-Type-Options",
}

// These request headers carry browser authentication and origin context. The
// public Authorization header is intentionally excluded: the verified private
// Rust bearer below is the only credential accepted by the sidecar. Cookies,
// origin, referer, and CSRF values still need to reach Rust so a rehearsal can
// exercise the same browser-session and CORS decisions as the Go owner.
var rehearsalRequestHeaders = []string{
	"Accept", "Content-Type", "Cookie", "Origin", "Referer", "X-CSRF-Token",
}

type rehearsalOperation struct {
	method   string
	template string
}

type rehearsalProxy struct {
	endpoint   *url.URL
	bearer     string
	operations []rehearsalOperation
	client     *http.Client
}

// ProxyTarget is the verified private Rust process surface consumed by the Go
// transport. It deliberately excludes lifecycle ownership.
type ProxyTarget interface {
	Endpoint() string
	BearerToken() string
	Profile() string
	Capabilities() []string
}

// ProxyOptions selects exact operations from one verified rehearsal process.
type ProxyOptions struct {
	Target     ProxyTarget
	Operations []string
	Timeout    time.Duration
}

// NewProxy returns a no-op middleware unless exact rehearsal operations were
// explicitly selected. Invalid private endpoints or capabilities fail closed.
func NewProxy(options ProxyOptions) gin.HandlerFunc {
	proxy := newRehearsalProxy(options.Target, options.Operations, options.Timeout)
	return func(c *gin.Context) {
		if proxy == nil || !proxy.selects(c.Request) {
			c.Next()
			return
		}
		proxy.forward(c)
	}
}

func newRehearsalProxy(target ProxyTarget, selected []string, timeout time.Duration) *rehearsalProxy {
	if target == nil || len(selected) == 0 {
		return nil
	}
	endpoint, err := url.Parse(strings.TrimSpace(target.Endpoint()))
	if err != nil || endpoint.Scheme != "http" || endpoint.Path != "" || !isLoopbackHost(endpoint.Hostname()) {
		panic("invalid verified Rust rehearsal endpoint")
	}
	capabilities := target.Capabilities()
	operations := make([]rehearsalOperation, 0, len(selected))
	for _, value := range selected {
		if !slices.Contains(capabilities, value) {
			panic(fmt.Sprintf("Rust rehearsal operation %q is not a verified capability", value))
		}
		operation, ok := parseRehearsalOperation(value)
		if !ok {
			panic(fmt.Sprintf("invalid Rust rehearsal operation %q", value))
		}
		operations = append(operations, operation)
	}
	if timeout <= 0 {
		timeout = defaultRehearsalProxyTimeout
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	return &rehearsalProxy{
		endpoint:   endpoint,
		bearer:     strings.TrimSpace(target.BearerToken()),
		operations: operations,
		client:     &http.Client{Transport: transport, Timeout: timeout},
	}
}

func parseRehearsalOperation(value string) (rehearsalOperation, bool) {
	method, template, ok := strings.Cut(strings.TrimSpace(value), " ")
	method, template = strings.TrimSpace(method), strings.TrimSpace(template)
	return rehearsalOperation{method: method, template: template},
		ok && method != "" && strings.HasPrefix(template, "/api/v1/")
}

func (p *rehearsalProxy) selects(request *http.Request) bool {
	if request == nil || request.URL == nil || !isOrdinaryJSONRequest(request) {
		return false
	}
	return slices.ContainsFunc(p.operations, func(operation rehearsalOperation) bool {
		return request.Method == operation.method && matchesPathTemplate(request.URL.Path, operation.template)
	})
}

func isOrdinaryJSONRequest(request *http.Request) bool {
	if strings.EqualFold(strings.TrimSpace(request.Header.Get("Upgrade")), "websocket") ||
		strings.Contains(strings.ToLower(request.Header.Get("Accept")), "text/event-stream") {
		return false
	}
	contentType, _, _ := strings.Cut(request.Header.Get("Content-Type"), ";")
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	return request.Body == nil || request.ContentLength == 0 || contentType == "application/json"
}

func matchesPathTemplate(path string, template string) bool {
	pathSegments := strings.Split(strings.Trim(path, "/"), "/")
	templateSegments := strings.Split(strings.Trim(template, "/"), "/")
	if len(pathSegments) != len(templateSegments) {
		return false
	}
	for index, expected := range templateSegments {
		if strings.HasPrefix(expected, "{") && strings.HasSuffix(expected, "}") {
			if pathSegments[index] == "" {
				return false
			}
			continue
		}
		if pathSegments[index] != expected {
			return false
		}
	}
	return true
}

func (p *rehearsalProxy) forward(c *gin.Context) {
	request := c.Request
	surface := verifiedAccessSurface(request)
	if surface == "" {
		httpserver.WriteError(c, http.StatusForbidden, "RUST_REHEARSAL_ACCESS_SURFACE_REQUIRED", "verified access surface is required")
		return
	}
	target := *p.endpoint
	target.Path = request.URL.Path
	target.RawPath = request.URL.RawPath
	target.RawQuery = request.URL.RawQuery
	forwarded, err := http.NewRequestWithContext(request.Context(), request.Method, target.String(), request.Body)
	if err != nil {
		httpserver.WriteError(c, http.StatusBadGateway, "RUST_REHEARSAL_PROXY_FAILED", "Rust rehearsal request could not be created")
		return
	}
	forwarded.ContentLength = request.ContentLength
	for _, name := range rehearsalRequestHeaders {
		copyRequestHeader(forwarded.Header, request.Header, name)
	}
	forwarded.Header.Set("Authorization", "Bearer "+p.bearer)
	forwarded.Header.Set(InternalProxyHeader, InternalProxyProtocol)
	forwarded.Header.Set(AccessSurfaceHeader, surface)
	forwarded.Header.Set("X-Request-ID", c.GetString("requestID"))

	response, err := p.client.Do(forwarded)
	if err != nil {
		status := http.StatusBadGateway
		code := "RUST_REHEARSAL_UNAVAILABLE"
		if errors.Is(err, contextDeadlineExceeded(request)) {
			status, code = http.StatusGatewayTimeout, "RUST_REHEARSAL_TIMEOUT"
		}
		httpserver.WriteError(c, status, code, "Rust rehearsal request failed; Go replay is disabled")
		return
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxRehearsalResponseBytes+1))
	if err != nil || len(body) > maxRehearsalResponseBytes {
		httpserver.WriteError(c, http.StatusBadGateway, "RUST_REHEARSAL_INVALID_RESPONSE", "Rust rehearsal response could not be accepted")
		return
	}
	for _, name := range rehearsalResponseHeaders {
		copyResponseHeader(c.Writer.Header(), response.Header, name)
	}
	c.Writer.Header().Set("X-Request-ID", c.GetString("requestID"))
	c.Status(response.StatusCode)
	if request.Method != http.MethodHead {
		_, _ = c.Writer.Write(body)
	}
	c.Abort()
}

func verifiedAccessSurface(request *http.Request) string {
	if middleware.IsRequestTrustedHost(request) {
		return "desktop"
	}
	if webaccess.IsAccessSurfaceRequest(request) {
		return "web"
	}
	return ""
}

func copyRequestHeader(destination http.Header, source http.Header, name string) {
	for _, value := range source.Values(name) {
		destination.Add(name, value)
	}
}

func copyResponseHeader(destination http.Header, source http.Header, name string) {
	for _, value := range source.Values(name) {
		destination.Add(name, value)
	}
}

func isLoopbackHost(host string) bool {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func contextDeadlineExceeded(request *http.Request) error {
	if request != nil && request.Context().Err() != nil {
		return request.Context().Err()
	}
	return context.DeadlineExceeded
}
