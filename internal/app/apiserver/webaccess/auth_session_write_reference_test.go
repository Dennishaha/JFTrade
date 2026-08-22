package webaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/middleware"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/security/passwordhash"
)

const (
	stage9AuthSessionWriteFixtureVersion = "stage9.auth-session-write.v1"
	stage9AuthSessionWriteOrigin         = "https://fixture.jftrade.local"
	stage9AuthSessionWriteTimestamp      = "2026-08-22T04:00:00Z"
	stage9AuthSessionWriteExpiry         = "Sat, 22 Aug 2026 16:00:00 GMT"
	stage9AuthSessionWritePassword       = "stage9-auth-session-write-password"
	stage9AuthSessionWriteCSRF           = "fixture-csrf-token"
	stage9AuthSessionWriteCookie         = "fixture-session-token"
	stage9AuthSessionWriteRequestID      = "fixture-auth-session-write-id"
)

var stage9AuthSessionWriteContractHeaders = []string{
	"access-control-allow-credentials",
	"access-control-allow-headers",
	"access-control-allow-methods",
	"access-control-allow-origin",
	"access-control-expose-headers",
	"cache-control",
	"content-type",
	"retry-after",
	"set-cookie",
	"vary",
	"x-request-id",
}

type stage9AuthSessionWriteFixture struct {
	Version string                              `json:"version"`
	Cases   []stage9AuthSessionWriteFixtureCase `json:"cases"`
}

type stage9AuthSessionWriteFixtureCase struct {
	Name     string                           `json:"name"`
	Requests []stage9AuthSessionWriteRequest  `json:"requests"`
	Expected []stage9AuthSessionWriteExpected `json:"expected"`
}

type stage9AuthSessionWriteRequest struct {
	Method         string                        `json:"method"`
	Path           string                        `json:"path"`
	Body           string                        `json:"body"`
	Headers        map[string]string             `json:"headers,omitempty"`
	RequestContext stage9AuthSessionWriteContext `json:"requestContext"`
	trusted        bool                          `json:"-"`
	requestContext context.Context               `json:"-"`
}

type stage9AuthSessionWriteExpected struct {
	Status          int               `json:"status"`
	ResponseHeaders map[string]string `json:"responseHeaders"`
	AbsentHeaders   []string          `json:"absentHeaders"`
	Envelope        json.RawMessage   `json:"envelope"`
	PortCall        bool              `json:"portCall"`
	PortError       string            `json:"portError,omitempty"`
}

type stage9AuthSessionWriteContext struct {
	DesktopTrusted       bool `json:"desktopTrusted"`
	BrowserAuthenticated bool `json:"browserAuthenticated"`
	OriginProvided       bool `json:"originProvided"`
	OriginAllowed        bool `json:"originAllowed"`
	CSRFValid            bool `json:"csrfValid"`
	WebAccessEnabled     bool `json:"webAccessEnabled"`
	WebAuthAvailable     bool `json:"webAuthAvailable"`
}

type stage9AuthSessionWritePortExpectation struct {
	Call  bool
	Error string
}

type stage9AuthSessionWriteRawResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

type stage9AuthSessionWriteHarness struct {
	auth   *Auth
	router *gin.Engine
}

// TestStage9AuthSessionWriteFixtureMatchesCurrentGoOwner freezes both auth
// mutation routes, including middleware precedence and cookie/header output.
func TestStage9AuthSessionWriteFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve auth-session-write fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../../../tests/fixtures/rust-migration/stage9/auth-session-write.json",
	)
	want := stage9AuthSessionWriteFixture{
		Version: stage9AuthSessionWriteFixtureVersion,
		Cases: []stage9AuthSessionWriteFixtureCase{
			stage9AuthSessionWriteLoginSuccess(t),
			stage9AuthSessionWriteLoginEmptyPassword(t),
			stage9AuthSessionWriteLoginInvalidJSON(t),
			stage9AuthSessionWriteLoginNullAndTrailingJSON(t),
			stage9AuthSessionWriteLoginOriginForbidden(t),
			stage9AuthSessionWriteLoginDisabled(t),
			stage9AuthSessionWriteLoginUnavailable(t),
			stage9AuthSessionWriteLoginTrustedDesktop(t),
			stage9AuthSessionWriteLoginRateLimited(t),
			stage9AuthSessionWriteLoginCanceled(t),
			stage9AuthSessionWriteLoginConfigurationChanged(t),
			stage9AuthSessionWriteLoginSessionCreationFailed(t),
			stage9AuthSessionWriteLogoutBrowser(t),
			stage9AuthSessionWriteLogoutUnauthenticated(t),
			stage9AuthSessionWriteLogoutNoOrigin(t),
			stage9AuthSessionWriteLogoutCSRFFailed(t),
			stage9AuthSessionWriteLogoutOriginForbidden(t),
			stage9AuthSessionWriteLogoutTrustedDesktop(t),
		},
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode auth-session-write fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write auth-session-write fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read auth-session-write fixture: %v", err)
	}
	var got stage9AuthSessionWriteFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode auth-session-write fixture: %v", err)
	}
	stage9AuthSessionWriteCompactFixture(&got)
	stage9AuthSessionWriteCompactFixture(&want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 auth-session-write fixture drifted from the Go owner: got=%#v want=%#v", got, want)
	}
}

func stage9AuthSessionWriteLoginSuccess(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, stage9AuthSessionWriteSettings(t))
	request := stage9AuthSessionWriteLoginRequest(`{"password":"` + stage9AuthSessionWritePassword + `"}`)
	return stage9AuthSessionWriteRunCase(t, harness, "login-success", []stage9AuthSessionWriteRequest{request}, []stage9AuthSessionWritePortExpectation{{Call: true}})
}

func stage9AuthSessionWriteLoginEmptyPassword(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, stage9AuthSessionWriteSettings(t))
	request := stage9AuthSessionWriteLoginRequest(`{}`)
	return stage9AuthSessionWriteRunCase(t, harness, "login-empty-password", []stage9AuthSessionWriteRequest{request}, []stage9AuthSessionWritePortExpectation{{Call: true, Error: "invalid-password"}})
}

func stage9AuthSessionWriteLoginInvalidJSON(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, stage9AuthSessionWriteSettings(t))
	request := stage9AuthSessionWriteLoginRequest(`{`)
	return stage9AuthSessionWriteRunCase(t, harness, "login-invalid-json", []stage9AuthSessionWriteRequest{request}, []stage9AuthSessionWritePortExpectation{{}})
}

func stage9AuthSessionWriteLoginNullAndTrailingJSON(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, stage9AuthSessionWriteSettings(t))
	requests := []stage9AuthSessionWriteRequest{
		stage9AuthSessionWriteLoginRequest("null"),
		stage9AuthSessionWriteLoginRequest(`{"password":"` + stage9AuthSessionWritePassword + `"} {"ignored":true}`),
	}
	return stage9AuthSessionWriteRunCase(t, harness, "login-null-and-trailing-json", requests, []stage9AuthSessionWritePortExpectation{
		{Call: true, Error: "invalid-password"},
		{Call: true},
	})
}

func stage9AuthSessionWriteLoginOriginForbidden(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, stage9AuthSessionWriteSettings(t))
	request := stage9AuthSessionWriteLoginRequest(`{"password":"` + stage9AuthSessionWritePassword + `"}`)
	request.Headers["Origin"] = "https://forbidden.example.test"
	request.RequestContext.OriginAllowed = false
	request.RequestContext.OriginProvided = true
	return stage9AuthSessionWriteRunCase(t, harness, "login-origin-forbidden", []stage9AuthSessionWriteRequest{request}, []stage9AuthSessionWritePortExpectation{{}})
}

func stage9AuthSessionWriteLoginDisabled(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, jfsettings.SecuritySettings{})
	request := stage9AuthSessionWriteLoginRequest(`{`)
	request.RequestContext.WebAccessEnabled = false
	request.RequestContext.WebAuthAvailable = false
	return stage9AuthSessionWriteRunCase(t, harness, "login-disabled-precedes-payload", []stage9AuthSessionWriteRequest{request}, []stage9AuthSessionWritePortExpectation{{}})
}

func stage9AuthSessionWriteLoginUnavailable(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, jfsettings.SecuritySettings{
		WebAccessEnabled: true,
		PasswordHash:     "invalid-password-hash",
	})
	request := stage9AuthSessionWriteLoginRequest(`{`)
	request.RequestContext.WebAccessEnabled = true
	request.RequestContext.WebAuthAvailable = false
	return stage9AuthSessionWriteRunCase(t, harness, "login-unavailable-precedes-payload", []stage9AuthSessionWriteRequest{request}, []stage9AuthSessionWritePortExpectation{{}})
}

func stage9AuthSessionWriteLoginTrustedDesktop(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, jfsettings.SecuritySettings{})
	request := stage9AuthSessionWriteLoginRequest("not-json")
	request.trusted = true
	request.RequestContext.DesktopTrusted = true
	request.RequestContext.WebAccessEnabled = false
	request.RequestContext.WebAuthAvailable = false
	return stage9AuthSessionWriteRunCase(t, harness, "login-trusted-desktop-bypasses-payload", []stage9AuthSessionWriteRequest{request}, []stage9AuthSessionWritePortExpectation{{}})
}

func stage9AuthSessionWriteLoginRateLimited(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, stage9AuthSessionWriteSettings(t))
	requests := make([]stage9AuthSessionWriteRequest, 0, 9)
	expectations := make([]stage9AuthSessionWritePortExpectation, 0, 9)
	for range 8 {
		request := stage9AuthSessionWriteLoginRequest(`{"password":"wrong-password"}`)
		requests = append(requests, request)
		expectations = append(expectations, stage9AuthSessionWritePortExpectation{Call: true, Error: "invalid-password"})
	}
	requests = append(requests, stage9AuthSessionWriteLoginRequest(`{"password":"`+stage9AuthSessionWritePassword+`"}`))
	expectations = append(expectations, stage9AuthSessionWritePortExpectation{})
	return stage9AuthSessionWriteRunCase(t, harness, "login-rate-limit-after-eight-failures", requests, expectations)
}

func stage9AuthSessionWriteLoginCanceled(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, stage9AuthSessionWriteSettings(t))
	cancelContext, cancel := context.WithCancel(context.Background())
	harness.auth.verifyPassword = func(_, _ string) (bool, error) {
		cancel()
		return false, errors.New("verification interrupted")
	}
	request := stage9AuthSessionWriteLoginRequest(`{"password":"` + stage9AuthSessionWritePassword + `"}`)
	request.requestContext = cancelContext
	return stage9AuthSessionWriteRunCase(t, harness, "login-canceled", []stage9AuthSessionWriteRequest{request}, []stage9AuthSessionWritePortExpectation{{Call: true, Error: "canceled"}})
}

func stage9AuthSessionWriteLoginConfigurationChanged(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, stage9AuthSessionWriteSettings(t))
	verificationStarted := make(chan struct{})
	continueVerification := make(chan struct{})
	harness.auth.verifyPassword = func(_, _ string) (bool, error) {
		close(verificationStarted)
		<-continueVerification
		return true, nil
	}
	request := stage9AuthSessionWriteLoginRequest(`{"password":"` + stage9AuthSessionWritePassword + `"}`)
	result := make(chan stage9AuthSessionWriteRawResponse, 1)
	done := make(chan struct{})
	go func() {
		result <- stage9AuthSessionWriteServeRequest(t, harness, request)
		close(done)
	}()
	select {
	case <-verificationStarted:
	case <-done:
		t.Fatal("configuration-change login finished before password verification")
	}
	replacementHash, err := passwordhash.Hash("replacement-auth-session-write-password")
	if err != nil {
		t.Fatalf("hash replacement auth-session-write password: %v", err)
	}
	harness.auth.Configure(jfsettings.SecuritySettings{
		WebAccessEnabled: true,
		PasswordHash:     replacementHash,
	})
	close(continueVerification)
	response := <-result
	return stage9AuthSessionWriteBuildCase(t, "login-configuration-changed", []stage9AuthSessionWriteRequest{request}, []stage9AuthSessionWritePortExpectation{{Call: true, Error: "configuration-changed"}}, []stage9AuthSessionWriteRawResponse{response})
}

func stage9AuthSessionWriteLoginSessionCreationFailed(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, stage9AuthSessionWriteSettings(t))
	harness.auth.verifyPassword = func(_, _ string) (bool, error) { return true, nil }
	harness.auth.generateSecret = func(int) (string, error) { return "", errors.New("entropy unavailable") }
	request := stage9AuthSessionWriteLoginRequest(`{"password":"` + stage9AuthSessionWritePassword + `"}`)
	return stage9AuthSessionWriteRunCase(t, harness, "login-session-creation-failed", []stage9AuthSessionWriteRequest{request}, []stage9AuthSessionWritePortExpectation{{Call: true, Error: "failed"}})
}

func stage9AuthSessionWriteLogoutBrowser(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, stage9AuthSessionWriteSettings(t))
	login := stage9AuthSessionWriteLoginRequest(`{"password":"` + stage9AuthSessionWritePassword + `"}`)
	loginResponse := stage9AuthSessionWriteServeRequest(t, harness, login)
	cookie := stage9AuthSessionWriteCookieFromHeader(t, loginResponse.Header.Get("Set-Cookie"))
	logout := stage9AuthSessionWriteRequest{
		Method: http.MethodPost,
		Path:   "/api/v1/auth/logout",
		Body:   "not-json",
		Headers: map[string]string{
			"Cookie":       cookie,
			"Origin":       stage9AuthSessionWriteOrigin,
			"X-CSRF-Token": stage9AuthSessionWriteCSRFValue(t, loginResponse),
		},
		RequestContext: stage9AuthSessionWriteContext{
			BrowserAuthenticated: true,
			OriginProvided:       true,
			OriginAllowed:        true,
			CSRFValid:            true,
			WebAccessEnabled:     true,
			WebAuthAvailable:     true,
		},
	}
	return stage9AuthSessionWriteRunCase(t, harness, "logout-browser-ignores-malformed-body", []stage9AuthSessionWriteRequest{logout}, []stage9AuthSessionWritePortExpectation{{Call: true}})
}

func stage9AuthSessionWriteLogoutUnauthenticated(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, stage9AuthSessionWriteSettings(t))
	request := stage9AuthSessionWriteLogoutRequest(stage9AuthSessionWriteContext{
		WebAccessEnabled: true,
		WebAuthAvailable: true,
	})
	return stage9AuthSessionWriteRunCase(t, harness, "logout-unauthenticated", []stage9AuthSessionWriteRequest{request}, []stage9AuthSessionWritePortExpectation{{}})
}

func stage9AuthSessionWriteLogoutNoOrigin(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, stage9AuthSessionWriteSettings(t))
	cookie, csrf := stage9AuthSessionWriteSeedSession(t, harness)
	request := stage9AuthSessionWriteLogoutRequest(stage9AuthSessionWriteContext{
		BrowserAuthenticated: true,
		CSRFValid:            true,
		WebAccessEnabled:     true,
		WebAuthAvailable:     true,
	})
	request.Headers["Cookie"] = cookie
	request.Headers["X-CSRF-Token"] = csrf
	return stage9AuthSessionWriteRunCase(t, harness, "logout-browser-requires-origin", []stage9AuthSessionWriteRequest{request}, []stage9AuthSessionWritePortExpectation{{}})
}

func stage9AuthSessionWriteLogoutCSRFFailed(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, stage9AuthSessionWriteSettings(t))
	cookie, _ := stage9AuthSessionWriteSeedSession(t, harness)
	request := stage9AuthSessionWriteLogoutRequest(stage9AuthSessionWriteContext{
		BrowserAuthenticated: true,
		OriginProvided:       true,
		OriginAllowed:        true,
		WebAccessEnabled:     true,
		WebAuthAvailable:     true,
	})
	request.Headers["Cookie"] = cookie
	request.Headers["X-CSRF-Token"] = "wrong-csrf-token"
	return stage9AuthSessionWriteRunCase(t, harness, "logout-browser-rejects-invalid-csrf", []stage9AuthSessionWriteRequest{request}, []stage9AuthSessionWritePortExpectation{{}})
}

func stage9AuthSessionWriteLogoutOriginForbidden(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, stage9AuthSessionWriteSettings(t))
	cookie, csrf := stage9AuthSessionWriteSeedSession(t, harness)
	request := stage9AuthSessionWriteLogoutRequest(stage9AuthSessionWriteContext{
		BrowserAuthenticated: true,
		OriginProvided:       true,
		OriginAllowed:        false,
		CSRFValid:            true,
		WebAccessEnabled:     true,
		WebAuthAvailable:     true,
	})
	request.Headers["Cookie"] = cookie
	request.Headers["Origin"] = "https://forbidden.example.test"
	request.Headers["X-CSRF-Token"] = csrf
	return stage9AuthSessionWriteRunCase(t, harness, "logout-origin-forbidden-precedes-session", []stage9AuthSessionWriteRequest{request}, []stage9AuthSessionWritePortExpectation{{}})
}

func stage9AuthSessionWriteLogoutTrustedDesktop(t *testing.T) stage9AuthSessionWriteFixtureCase {
	harness := stage9AuthSessionWriteNewHarness(t, jfsettings.SecuritySettings{})
	request := stage9AuthSessionWriteLogoutRequest(stage9AuthSessionWriteContext{})
	request.trusted = true
	request.RequestContext.DesktopTrusted = true
	return stage9AuthSessionWriteRunCase(t, harness, "logout-trusted-desktop-bypasses-csrf", []stage9AuthSessionWriteRequest{request}, []stage9AuthSessionWritePortExpectation{{Call: true}})
}

func stage9AuthSessionWriteSettings(t *testing.T) jfsettings.SecuritySettings {
	t.Helper()
	hash, err := passwordhash.Hash(stage9AuthSessionWritePassword)
	if err != nil {
		t.Fatalf("hash auth-session-write password: %v", err)
	}
	return jfsettings.SecuritySettings{
		WebAccessEnabled:    true,
		PublicAccessEnabled: true,
		PasswordHash:        hash,
	}
}

func stage9AuthSessionWriteNewHarness(t *testing.T, settings jfsettings.SecuritySettings) *stage9AuthSessionWriteHarness {
	t.Helper()
	auth := NewAuth(settings)
	auth.ConfigureOrigins(stage9AuthSessionWriteOrigin)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Header("X-Request-ID", stage9AuthSessionWriteRequestID)
		c.Next()
	})
	router.Use(middleware.CORS(auth))
	router.Use(middleware.Auth(auth, auth, nil, auth))
	router.POST(stage9AuthSessionWriteLoginPath(), auth.Login)
	router.POST(stage9AuthSessionWriteLogoutPath(), auth.Logout)
	t.Cleanup(auth.Close)
	return &stage9AuthSessionWriteHarness{auth: auth, router: router}
}

func stage9AuthSessionWriteLoginPath() string { return "/api/v1/auth/login" }

func stage9AuthSessionWriteLogoutPath() string { return "/api/v1/auth/logout" }

func stage9AuthSessionWriteLoginRequest(body string) stage9AuthSessionWriteRequest {
	return stage9AuthSessionWriteRequest{
		Method: http.MethodPost,
		Path:   stage9AuthSessionWriteLoginPath(),
		Body:   body,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Origin":       stage9AuthSessionWriteOrigin,
		},
		RequestContext: stage9AuthSessionWriteContext{
			OriginProvided:   true,
			OriginAllowed:    true,
			WebAccessEnabled: true,
			WebAuthAvailable: true,
		},
	}
}

func stage9AuthSessionWriteLogoutRequest(context stage9AuthSessionWriteContext) stage9AuthSessionWriteRequest {
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if context.OriginProvided {
		headers["Origin"] = stage9AuthSessionWriteOrigin
	}
	return stage9AuthSessionWriteRequest{
		Method:         http.MethodPost,
		Path:           stage9AuthSessionWriteLogoutPath(),
		Body:           "",
		Headers:        headers,
		RequestContext: context,
	}
}

func stage9AuthSessionWriteSeedSession(t *testing.T, harness *stage9AuthSessionWriteHarness) (string, string) {
	t.Helper()
	response := stage9AuthSessionWriteServeRequest(t, harness, stage9AuthSessionWriteLoginRequest(`{"password":"`+stage9AuthSessionWritePassword+`"}`))
	return stage9AuthSessionWriteCookieFromHeader(t, response.Header.Get("Set-Cookie")), stage9AuthSessionWriteCSRFValue(t, response)
}

func stage9AuthSessionWriteCookieFromHeader(t *testing.T, header string) string {
	t.Helper()
	parts := strings.Split(header, ";")
	if len(parts) == 0 || !strings.HasPrefix(parts[0], "jftrade_web_session=") {
		t.Fatalf("session cookie = %q", header)
	}
	return parts[0]
}

func stage9AuthSessionWriteCSRFValue(t *testing.T, response stage9AuthSessionWriteRawResponse) string {
	t.Helper()
	var envelope struct {
		Data struct {
			CSRFToken string `json:"csrfToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body, &envelope); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if envelope.Data.CSRFToken == "" {
		t.Fatal("login response did not include csrf token")
	}
	return envelope.Data.CSRFToken
}

func stage9AuthSessionWriteRunCase(
	t *testing.T,
	harness *stage9AuthSessionWriteHarness,
	name string,
	requests []stage9AuthSessionWriteRequest,
	expectations []stage9AuthSessionWritePortExpectation,
) stage9AuthSessionWriteFixtureCase {
	t.Helper()
	responses := make([]stage9AuthSessionWriteRawResponse, 0, len(requests))
	for _, request := range requests {
		responses = append(responses, stage9AuthSessionWriteServeRequest(t, harness, request))
	}
	return stage9AuthSessionWriteBuildCase(t, name, requests, expectations, responses)
}

func stage9AuthSessionWriteServeRequest(
	t *testing.T,
	harness *stage9AuthSessionWriteHarness,
	request stage9AuthSessionWriteRequest,
) stage9AuthSessionWriteRawResponse {
	t.Helper()
	requestContext := request.requestContext
	if requestContext == nil {
		requestContext = context.Background()
	}
	httpRequest := httptest.NewRequestWithContext(
		requestContext,
		request.Method,
		request.Path,
		bytes.NewBufferString(request.Body),
	)
	httpRequest.RemoteAddr = "127.0.0.1:42000"
	for name, value := range request.Headers {
		httpRequest.Header.Set(name, value)
	}
	if request.trusted {
		httpRequest = middleware.MarkRequestTrustedHost(httpRequest)
	}
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, httpRequest)
	return stage9AuthSessionWriteRawResponse{
		Status: recorder.Code,
		Header: recorder.Header().Clone(),
		Body:   append([]byte(nil), recorder.Body.Bytes()...),
	}
}

func stage9AuthSessionWriteBuildCase(
	t *testing.T,
	name string,
	requests []stage9AuthSessionWriteRequest,
	expectations []stage9AuthSessionWritePortExpectation,
	responses []stage9AuthSessionWriteRawResponse,
) stage9AuthSessionWriteFixtureCase {
	t.Helper()
	if len(requests) != len(expectations) || len(requests) != len(responses) {
		t.Fatalf("auth-session-write case %s lengths: requests=%d expectations=%d responses=%d", name, len(requests), len(expectations), len(responses))
	}
	fixtureRequests := make([]stage9AuthSessionWriteRequest, len(requests))
	fixtureExpected := make([]stage9AuthSessionWriteExpected, len(requests))
	for index := range requests {
		fixtureRequests[index] = stage9AuthSessionWriteFixtureRequest(requests[index])
		fixtureExpected[index] = stage9AuthSessionWriteNormalizeResponse(t, responses[index], expectations[index])
	}
	return stage9AuthSessionWriteFixtureCase{
		Name:     name,
		Requests: fixtureRequests,
		Expected: fixtureExpected,
	}
}

func stage9AuthSessionWriteFixtureRequest(request stage9AuthSessionWriteRequest) stage9AuthSessionWriteRequest {
	request.Headers = stage9AuthSessionWriteNormalizeRequestHeaders(request.Headers)
	request.requestContext = nil
	request.trusted = false
	return request
}

func stage9AuthSessionWriteNormalizeResponse(
	t *testing.T,
	response stage9AuthSessionWriteRawResponse,
	port stage9AuthSessionWritePortExpectation,
) stage9AuthSessionWriteExpected {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(response.Body, &envelope); err != nil {
		t.Fatalf("decode auth-session-write response: %v (%s)", err, response.Body)
	}
	envelope["timestamp"] = stage9AuthSessionWriteTimestamp
	if data, ok := envelope["data"].(map[string]any); ok {
		if _, ok := data["csrfToken"]; ok && data["csrfToken"] != "" {
			data["csrfToken"] = stage9AuthSessionWriteCSRF
		}
		if _, ok := data["expiresAt"]; ok {
			data["expiresAt"] = "fixture-time"
		}
	}
	normalizedEnvelope, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("encode auth-session-write response: %v", err)
	}
	expected := stage9AuthSessionWriteExpected{
		Status:          response.Status,
		ResponseHeaders: map[string]string{},
		AbsentHeaders:   []string{},
		Envelope:        normalizedEnvelope,
		PortCall:        port.Call,
		PortError:       port.Error,
	}
	for _, name := range stage9AuthSessionWriteContractHeaders {
		values := response.Header.Values(http.CanonicalHeaderKey(name))
		if len(values) == 0 {
			expected.AbsentHeaders = append(expected.AbsentHeaders, name)
			continue
		}
		value := strings.Join(values, "\n")
		if name == "set-cookie" {
			value = stage9AuthSessionWriteNormalizeCookie(value)
		}
		if name == "retry-after" {
			value = "300"
		}
		expected.ResponseHeaders[name] = value
	}
	return expected
}

func stage9AuthSessionWriteNormalizeCookie(value string) string {
	parts := strings.Split(value, "; ")
	if len(parts) == 0 {
		return value
	}
	if strings.HasPrefix(parts[0], "jftrade_web_session=") && parts[0] != "jftrade_web_session=" {
		parts[0] = "jftrade_web_session=" + stage9AuthSessionWriteCookie
		for index, part := range parts {
			if strings.HasPrefix(part, "Expires=") {
				parts[index] = "Expires=" + stage9AuthSessionWriteExpiry
			}
		}
	}
	return strings.Join(parts, "; ")
}

func stage9AuthSessionWriteNormalizeRequestHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(headers))
	for name, value := range headers {
		switch strings.ToLower(name) {
		case "cookie":
			if strings.HasPrefix(value, "jftrade_web_session=") {
				value = "jftrade_web_session=" + stage9AuthSessionWriteCookie
			}
		case "x-csrf-token":
			if value != "" && value != "wrong-csrf-token" {
				value = stage9AuthSessionWriteCSRF
			}
		}
		normalized[name] = value
	}
	return normalized
}

func stage9AuthSessionWriteCompactFixture(fixture *stage9AuthSessionWriteFixture) {
	for caseIndex := range fixture.Cases {
		for expectedIndex := range fixture.Cases[caseIndex].Expected {
			var compacted bytes.Buffer
			envelope := fixture.Cases[caseIndex].Expected[expectedIndex].Envelope
			if json.Compact(&compacted, envelope) == nil {
				fixture.Cases[caseIndex].Expected[expectedIndex].Envelope = compacted.Bytes()
			}
		}
	}
}
