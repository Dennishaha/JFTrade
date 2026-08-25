package rustrehearsal

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/middleware"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/webaccess"
)

const requestIDHeader = "X-Request-ID"

type rehearsalProxyTargetFixture struct {
	endpoint     string
	capabilities []string
}

func (f rehearsalProxyTargetFixture) Endpoint() string       { return f.endpoint }
func (f rehearsalProxyTargetFixture) BearerToken() string    { return "private-rust-bearer" }
func (f rehearsalProxyTargetFixture) Profile() string        { return "read-only-shadow.v1" }
func (f rehearsalProxyTargetFixture) Capabilities() []string { return f.capabilities }

func TestRehearsalProxyStaysDisabledWithoutVerifiedTarget(t *testing.T) {
	t.Parallel()
	if proxy := newRehearsalProxy(nil, []string{"GET /api/v1/catalog"}, time.Second); proxy != nil {
		t.Fatal("proxy enabled without a verified Rust target")
	}
}

func TestRehearsalProxyForwardsExactOperationAfterVerifiedSurface(t *testing.T) {
	t.Parallel()
	const operation = "POST /api/v1/widgets/{id}"
	var rustCalls atomic.Int32
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rustCalls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/widgets/alpha" || r.URL.RawQuery != "view=full" {
			t.Errorf("unexpected Rust target request: %s %s", r.Method, r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer private-rust-bearer" {
			t.Errorf("private authorization = %q", got)
		}
		if got := r.Header.Get(InternalProxyHeader); got != InternalProxyProtocol {
			t.Errorf("internal proxy protocol = %q", got)
		}
		if got := r.Header.Get(AccessSurfaceHeader); got != "desktop" {
			t.Errorf("access surface = %q", got)
		}
		if got := r.Header.Get(requestIDHeader); got != "stable-request-7" {
			t.Errorf("request ID = %q", got)
		}
		if got := r.Header.Get("Cookie"); got != "jftrade_web_session=browser-session" {
			t.Errorf("browser session cookie = %q", got)
		}
		if got := r.Header.Get("Origin"); got != "https://console.example" {
			t.Errorf("browser origin = %q", got)
		}
		if got := r.Header.Get("Referer"); got != "https://console.example/settings" {
			t.Errorf("browser referer = %q", got)
		}
		if got := r.Header.Get("X-CSRF-Token"); got != "browser-csrf" {
			t.Errorf("browser CSRF token = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer private-rust-bearer" {
			t.Errorf("public authorization replaced private bearer = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != `{"enabled":true}` {
			t.Errorf("body = %q, err = %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set(requestIDHeader, "rust-must-not-replace-go-id")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer rust.Close()

	var goCalls atomic.Int32
	router := rehearsalProxyTestRouter(rehearsalProxyTargetFixture{rust.URL, []string{operation}}, []string{operation}, time.Second, &goCalls)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/widgets/alpha?view=full", strings.NewReader(`{"enabled":true}`))
	request.Header.Set("Authorization", "Bearer public-desktop-token")
	request.Header.Set("Cookie", "jftrade_web_session=browser-session")
	request.Header.Set("Origin", "https://console.example")
	request.Header.Set("Referer", "https://console.example/settings")
	request.Header.Set("X-CSRF-Token", "browser-csrf")
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set(requestIDHeader, "stable-request-7")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || response.Body.String() != `{"ok":true}` {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	if response.Header().Get(requestIDHeader) != "stable-request-7" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("response headers = %#v", response.Header())
	}
	if rustCalls.Load() != 1 || goCalls.Load() != 0 {
		t.Fatalf("calls: Rust=%d Go=%d", rustCalls.Load(), goCalls.Load())
	}
}

func TestRehearsalProxyForwardsSelectedSSEOperationAndHeaders(t *testing.T) {
	t.Parallel()
	const operation = "POST /api/v1/adk/chat/stream"
	const sseBody = "retry: 3000\n\nid: 1\ndata: {\"type\":\"final\"}\n\n"
	var rustCalls atomic.Int32
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rustCalls.Add(1)
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("SSE Accept = %q", got)
		}
		if got := r.Header.Get("Cookie"); got != "jftrade_web_session=browser-session" {
			t.Errorf("SSE cookie = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != `{"message":"hello"}` {
			t.Errorf("SSE body = %q, err = %v", body, err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-ADK-Stream-ID", "stream-fixture")
		w.Header().Set("X-ADK-Stream-Idle-Timeout-Ms", "420000")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, sseBody)
	}))
	defer rust.Close()

	var goCalls atomic.Int32
	router := rehearsalProxyTestRouter(
		rehearsalProxyTargetFixture{rust.URL, []string{operation}},
		[]string{operation},
		time.Second,
		&goCalls,
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/adk/chat/stream",
		strings.NewReader(`{"message":"hello"}`),
	)
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", "jftrade_web_session=browser-session")
	request.Header.Set("X-Request-ID", "sse-request-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("SSE response status = %d; body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "text/event-stream" ||
		response.Header().Get("Connection") != "keep-alive" ||
		response.Header().Get("X-ADK-Stream-ID") != "stream-fixture" ||
		response.Header().Get("X-ADK-Stream-Idle-Timeout-Ms") != "420000" {
		t.Fatalf("SSE response headers = %#v", response.Header())
	}
	if response.Body.String() != sseBody {
		t.Fatalf("SSE response body = %q", response.Body.String())
	}
	if rustCalls.Load() != 1 || goCalls.Load() != 0 {
		t.Fatalf("calls: Rust=%d Go=%d", rustCalls.Load(), goCalls.Load())
	}
}

func TestRehearsalProxyDoesNotSelectMethodPathOrStreamingNearMisses(t *testing.T) {
	t.Parallel()
	if matchesPathTemplate("/api/v1//details", "/api/v1/{id}/details") {
		t.Fatal("empty path parameter matched")
	}
	if matchesPathTemplate("/api/v2/catalog", "/api/v1/catalog") {
		t.Fatal("literal path mismatch matched")
	}
	const operation = "POST /api/v1/widgets/{id}"
	var rustCalls atomic.Int32
	rust := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { rustCalls.Add(1) }))
	defer rust.Close()
	var goCalls atomic.Int32
	router := rehearsalProxyTestRouter(rehearsalProxyTargetFixture{rust.URL, []string{operation}}, []string{operation}, time.Second, &goCalls)

	requests := []*http.Request{
		httptest.NewRequest(http.MethodPut, "/api/v1/widgets/alpha", strings.NewReader(`{}`)),
		httptest.NewRequest(http.MethodPost, "/api/v1/widgets/alpha/extra", strings.NewReader(`{}`)),
		httptest.NewRequest(http.MethodPost, "/api/v1/widgets/alpha", strings.NewReader("form=true")),
		httptest.NewRequest(http.MethodPost, "/api/v1/widgets/alpha", nil),
	}
	requests[0].Header.Set("Content-Type", "application/json")
	requests[1].Header.Set("Content-Type", "application/json")
	requests[2].Header.Set("Content-Type", "application/x-www-form-urlencoded")
	requests[3].Header.Set("Accept", "text/event-stream")
	for _, request := range requests {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusAccepted {
			t.Fatalf("near miss %s %s returned %d", request.Method, request.URL, response.Code)
		}
	}
	if rustCalls.Load() != 0 || goCalls.Load() != int32(len(requests)) {
		t.Fatalf("calls: Rust=%d Go=%d", rustCalls.Load(), goCalls.Load())
	}
}

func TestRehearsalProxyConfigurationAndAccessSurfaceFailClosed(t *testing.T) {
	t.Parallel()
	const operation = "GET /api/v1/catalog"
	expectPanic := func(name string, options ProxyOptions) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("invalid proxy configuration did not panic")
				}
			}()
			_ = NewProxy(options)
		})
	}
	expectPanic("non-loopback endpoint", ProxyOptions{
		Target: rehearsalProxyTargetFixture{"http://0.0.0.0:3000", []string{operation}}, Operations: []string{operation},
	})
	expectPanic("unverified capability", ProxyOptions{
		Target: rehearsalProxyTargetFixture{"http://127.0.0.1:3000", nil}, Operations: []string{operation},
	})
	expectPanic("invalid operation", ProxyOptions{
		Target: rehearsalProxyTargetFixture{"http://127.0.0.1:3000", []string{"invalid"}}, Operations: []string{"invalid"},
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/catalog", nil)
	if got := verifiedAccessSurface(webaccess.WithAccessSurface(request)); got != "web" {
		t.Fatalf("Web access surface = %q", got)
	}
	var goCalls atomic.Int32
	router := gin.New()
	router.Use(NewProxy(ProxyOptions{
		Target: rehearsalProxyTargetFixture{"http://127.0.0.1:3000", []string{operation}}, Operations: []string{operation},
	}))
	router.GET("/api/v1/catalog", func(c *gin.Context) {
		goCalls.Add(1)
		c.Status(http.StatusAccepted)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/catalog", nil))
	if response.Code != http.StatusForbidden || goCalls.Load() != 0 {
		t.Fatalf("unverified surface response = %d, Go calls = %d", response.Code, goCalls.Load())
	}
}

func TestRehearsalProxyNeverReplaysRustFailureToGo(t *testing.T) {
	t.Parallel()
	const operation = "GET /api/v1/catalog"
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		timeout    time.Duration
		wantStatus int
	}{
		{name: "Rust error", handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"ok":false}`))
		}, timeout: time.Second, wantStatus: http.StatusServiceUnavailable},
		{name: "timeout", handler: func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}, timeout: 20 * time.Millisecond, wantStatus: http.StatusGatewayTimeout},
		{name: "oversized response", handler: func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(make([]byte, maxRehearsalResponseBytes+1))
		}, timeout: time.Second, wantStatus: http.StatusBadGateway},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rust := httptest.NewServer(test.handler)
			defer rust.Close()
			var goCalls atomic.Int32
			router := rehearsalProxyTestRouter(rehearsalProxyTargetFixture{rust.URL, []string{operation}}, []string{operation}, test.timeout, &goCalls)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/catalog", nil))
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.wantStatus, response.Body.String())
			}
			if goCalls.Load() != 0 {
				t.Fatalf("Go handler replayed %d times", goCalls.Load())
			}
		})
	}
}

func rehearsalProxyTestRouter(target ProxyTarget, selected []string, timeout time.Duration, goCalls *atomic.Int32) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("requestID", c.GetHeader(requestIDHeader))
		c.Writer.Header().Set(requestIDHeader, c.GetString("requestID"))
		c.Next()
	})
	router.Use(func(c *gin.Context) {
		c.Request = middleware.MarkRequestTrustedHost(c.Request)
		c.Next()
	})
	router.Use(NewProxy(ProxyOptions{Target: target, Operations: selected, Timeout: timeout}))
	handler := func(c *gin.Context) {
		goCalls.Add(1)
		c.Status(http.StatusAccepted)
	}
	router.Any("/api/v1/widgets/:id", handler)
	router.Any("/api/v1/catalog", handler)
	router.NoRoute(handler)
	return router
}

func TestRehearsalProxyPropagatesCallerCancellation(t *testing.T) {
	t.Parallel()
	const operation = "GET /api/v1/catalog"
	started := make(chan struct{})
	canceled := make(chan struct{})
	rust := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer rust.Close()
	var goCalls atomic.Int32
	router := rehearsalProxyTestRouter(rehearsalProxyTargetFixture{rust.URL, []string{operation}}, []string{operation}, time.Second, &goCalls)
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/api/v1/catalog", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(httptest.NewRecorder(), request)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Rust request did not start")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("Rust request context was not canceled")
	}
	<-done
	if goCalls.Load() != 0 {
		t.Fatal("canceled Rust request replayed to Go")
	}
}
