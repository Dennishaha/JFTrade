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
		if got := r.Header.Get("Cookie"); got != "" {
			t.Errorf("public cookie leaked to Rust: %q", got)
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
	request.Header.Set("Cookie", "jftrade_web_session=must-not-forward")
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

func TestRehearsalProxyDoesNotSelectMethodPathOrStreamingNearMisses(t *testing.T) {
	t.Parallel()
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
