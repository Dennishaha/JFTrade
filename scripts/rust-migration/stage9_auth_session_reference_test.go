package rustmigration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/security/passwordhash"
)

const (
	stage9AuthSessionFixtureVersion = "stage9.auth-session.v1"
	stage9AuthSessionFixtureOrigin  = "https://fixture.jftrade.local"
)

var stage9AuthSessionContractHeaderNames = []string{
	"access-control-allow-credentials",
	"access-control-allow-headers",
	"access-control-allow-methods",
	"access-control-allow-origin",
	"access-control-expose-headers",
	"cache-control",
	"content-type",
	"vary",
	"x-request-id",
}

type stage9AuthSessionFixture struct {
	Version string                  `json:"version"`
	Cases   []stage9AuthSessionCase `json:"cases"`
}

type stage9AuthSessionCase struct {
	Name            string                   `json:"name"`
	Method          string                   `json:"method"`
	RequestPath     string                   `json:"requestPath"`
	RequestContext  stage9AuthSessionContext `json:"requestContext"`
	ExpectedStatus  int                      `json:"expectedStatus"`
	ResponseHeaders map[string]string        `json:"responseHeaders"`
	AbsentHeaders   []string                 `json:"absentHeaders"`
	Data            json.RawMessage          `json:"data,omitempty"`
	ErrorCode       string                   `json:"errorCode,omitempty"`
	ErrorMessage    string                   `json:"errorMessage,omitempty"`
}

type stage9AuthSessionContext struct {
	DesktopTrusted       bool `json:"desktopTrusted"`
	BrowserAuthenticated bool `json:"browserAuthenticated"`
	OriginProvided       bool `json:"originProvided"`
	OriginAllowed        bool `json:"originAllowed"`
}

// TestStage9AuthSessionFixtureMatchesCurrentGoOwner freezes the public
// session projection without copying Go's browser-session owner into Rust.
func TestStage9AuthSessionFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve auth-session fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/auth-session.json",
	)

	password := "stage9 auth session fixture password"
	hash, err := passwordhash.Hash(password)
	if err != nil {
		t.Fatalf("hash auth-session fixture password: %v", err)
	}
	settings := jfsettings.SecuritySettings{
		WebAccessEnabled: true,
		PasswordHash:     hash,
	}
	browserHandler := stage9AuthSessionHandler(t, settings, servercore.SidecarOptions{})
	browserServer := httptest.NewServer(browserHandler)
	t.Cleanup(browserServer.Close)
	browserHandler.ConfigureAuthOrigins(browserServer.URL)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	browserClient := &http.Client{Jar: jar}
	stage9AuthSessionLogin(t, browserClient, browserServer.URL, password)

	desktopToken := "stage9-auth-session-desktop-token"
	desktopHandler := stage9AuthSessionHandler(t, settings, servercore.SidecarOptions{
		DesktopMode:     true,
		DesktopAPIToken: desktopToken,
	})
	desktopServer := httptest.NewServer(desktopHandler)
	t.Cleanup(desktopServer.Close)
	desktopHandler.ConfigureAuthOrigins(desktopServer.URL)

	cases := []struct {
		name    string
		server  string
		client  *http.Client
		headers map[string]string
		context stage9AuthSessionContext
	}{
		{
			name:   "unauthenticated",
			server: browserServer.URL,
			client: http.DefaultClient,
			context: stage9AuthSessionContext{
				OriginAllowed: true,
			},
		},
		{
			name:   "browser-session",
			server: browserServer.URL,
			client: browserClient,
			context: stage9AuthSessionContext{
				BrowserAuthenticated: true,
				OriginAllowed:        true,
			},
		},
		{
			name:   "browser-session-allowed-origin",
			server: browserServer.URL,
			client: browserClient,
			headers: map[string]string{
				"Origin": browserServer.URL,
			},
			context: stage9AuthSessionContext{
				BrowserAuthenticated: true,
				OriginProvided:       true,
				OriginAllowed:        true,
			},
		},
		{
			name:   "desktop-trusted",
			server: desktopServer.URL,
			client: http.DefaultClient,
			headers: map[string]string{
				"Authorization": "Bearer " + desktopToken,
			},
			context: stage9AuthSessionContext{
				DesktopTrusted: true,
				OriginAllowed:  true,
			},
		},
		{
			name:   "origin-forbidden",
			server: browserServer.URL,
			client: http.DefaultClient,
			headers: map[string]string{
				"Origin": "https://forbidden.example.test",
			},
			context: stage9AuthSessionContext{
				OriginProvided: true,
				OriginAllowed:  false,
			},
		},
	}

	want := stage9AuthSessionFixture{
		Version: stage9AuthSessionFixtureVersion,
		Cases:   make([]stage9AuthSessionCase, 0, len(cases)),
	}
	for _, testCase := range cases {
		response := stage9AuthSessionRequest(t, testCase.client, testCase.server, testCase.headers)
		entry := stage9AuthSessionCase{
			Name:            testCase.name,
			Method:          http.MethodGet,
			RequestPath:     "/api/v1/auth/session",
			RequestContext:  testCase.context,
			ExpectedStatus:  response.StatusCode,
			ResponseHeaders: stage9AuthSessionResponseHeaders(response.Header),
			AbsentHeaders:   stage9AuthSessionAbsentHeaders(response.Header),
		}
		var envelope struct {
			Data  json.RawMessage `json:"data"`
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			_ = response.Body.Close()
			t.Fatalf("decode %s auth-session response: %v", testCase.name, err)
		}
		if err := response.Body.Close(); err != nil {
			t.Fatalf("close %s auth-session response: %v", testCase.name, err)
		}
		if envelope.Error != nil {
			entry.ErrorCode = envelope.Error.Code
			entry.ErrorMessage = envelope.Error.Message
		} else {
			entry.Data = stage9NormalizeAuthSessionData(t, envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}

	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode auth-session fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write auth-session fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read auth-session fixture: %v", err)
	}
	var got stage9AuthSessionFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode auth-session fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactAuthSessionJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactAuthSessionJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 auth-session fixture drifted from the Go owner: got=%#v want=%#v", got, want)
	}
}

func stage9AuthSessionHandler(
	t *testing.T,
	settings jfsettings.SecuritySettings,
	options servercore.SidecarOptions,
) servercore.SidecarHandler {
	t.Helper()
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("create auth-session settings store: %v", err)
	}
	if err := store.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderFutu); err != nil {
		t.Fatalf("set auth-session fixture provider: %v", err)
	}
	handler := servercore.NewSidecarHandlerWithOptions(store, options)
	handler.ApplySecuritySettings(settings)
	t.Cleanup(func() {
		if err := handler.Close(); err != nil {
			t.Errorf("close auth-session handler: %v", err)
		}
	})
	return handler
}

func stage9AuthSessionLogin(t *testing.T, client *http.Client, baseURL, password string) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"password": password})
	if err != nil {
		t.Fatalf("encode auth-session login: %v", err)
	}
	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		baseURL+"/api/v1/auth/login",
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("build auth-session login: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", baseURL)
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("perform auth-session login: %v", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("auth-session login status = %d", response.StatusCode)
	}
}

func stage9AuthSessionRequest(
	t *testing.T,
	client *http.Client,
	baseURL string,
	headers map[string]string,
) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		baseURL+"/api/v1/auth/session",
		nil,
	)
	if err != nil {
		t.Fatalf("build auth-session request: %v", err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	request.Header.Set("X-Request-ID", "fixture-auth-session-id")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("perform auth-session request: %v", err)
	}
	return response
}

func stage9NormalizeAuthSessionData(t *testing.T, data json.RawMessage) json.RawMessage {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode auth-session data: %v", err)
	}
	if csrf, ok := value["csrfToken"].(string); ok && csrf != "" {
		value["csrfToken"] = "fixture-csrf-token"
	}
	if expiresAt, ok := value["expiresAt"].(string); ok && expiresAt != "" {
		value["expiresAt"] = "fixture-time"
	}
	contents, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode auth-session data: %v", err)
	}
	return contents
}

func compactAuthSessionJSON(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, data); err != nil {
		return data
	}
	return compacted.Bytes()
}

// stage9AuthSessionResponseHeaders captures every application-controlled
// auth-session response header. Transport framing headers such as Date and
// Content-Length are intentionally outside this HTTP contract fixture.
func stage9AuthSessionResponseHeaders(headers http.Header) map[string]string {
	values := make(map[string]string, len(stage9AuthSessionContractHeaderNames))
	for _, name := range stage9AuthSessionContractHeaderNames {
		value := headers.Get(name)
		if value == "" {
			continue
		}
		if name == "access-control-allow-origin" {
			value = stage9AuthSessionFixtureOrigin
		}
		values[name] = value
	}
	return values
}

func stage9AuthSessionAbsentHeaders(headers http.Header) []string {
	absent := make([]string, 0, len(stage9AuthSessionContractHeaderNames))
	for _, name := range stage9AuthSessionContractHeaderNames {
		if headers.Get(name) == "" {
			absent = append(absent, name)
		}
	}
	return absent
}
