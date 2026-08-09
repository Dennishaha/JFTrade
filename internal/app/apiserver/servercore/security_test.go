package servercore

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/security/passwordhash"
)

const testWebPassword = "correct horse battery staple"

func webSecuritySettings(t *testing.T, public bool) jfsettings.SecuritySettings {
	t.Helper()
	hash, err := passwordhash.Hash(testWebPassword)
	if err != nil {
		t.Fatalf("passwordhash.Hash: %v", err)
	}
	return jfsettings.SecuritySettings{
		WebAccessEnabled:    true,
		PublicAccessEnabled: public,
		PasswordHash:        hash,
	}
}

func webSecuritySettingsForPassword(t *testing.T, password string, public bool) jfsettings.SecuritySettings {
	t.Helper()
	hash, err := passwordhash.Hash(password)
	if err != nil {
		t.Fatalf("passwordhash.Hash: %v", err)
	}
	return jfsettings.SecuritySettings{WebAccessEnabled: true, PublicAccessEnabled: public, PasswordHash: hash}
}

func TestSecurityChangeCancelsExistingWebStream(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	jftradeCheckTestError(t, err)
	server := newTestServer(t, store)
	server.auth.SetEnforceAccess(true)
	server.ApplySecuritySettings(webSecuritySettings(t, false))
	streamStarted := make(chan struct{})
	streamClosed := make(chan struct{})
	server.router.GET("/api/v1/test/security-stream", func(c *gin.Context) {
		close(streamStarted)
		c.Header("Content-Type", "text/event-stream")
		c.Status(http.StatusOK)
		c.Writer.Flush()
		<-c.Request.Context().Done()
		close(streamClosed)
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)
	server.auth.ConfigureOrigins(srv.URL)
	client := newCookieClient(t)
	loginWeb(t, client, srv.URL, testWebPassword)

	responseResult := make(chan *http.Response, 1)
	errorResult := make(chan error, 1)
	go func() {
		response, requestErr := client.Get(srv.URL + "/api/v1/test/security-stream")
		if requestErr != nil {
			errorResult <- requestErr
			return
		}
		responseResult <- response
	}()

	select {
	case <-streamStarted:
	case requestErr := <-errorResult:
		t.Fatalf("open stream: %v", requestErr)
	case <-time.After(5 * time.Second):
		t.Fatal("Web stream did not start")
	}
	server.ApplySecuritySettings(webSecuritySettingsForPassword(t, "replacement browser password", false))
	select {
	case <-streamClosed:
	case <-time.After(5 * time.Second):
		t.Fatal("security change did not cancel Web stream")
	}
	select {
	case response := <-responseResult:
		jftradeCheckTestError(t, response.Body.Close())
	case requestErr := <-errorResult:
		t.Fatalf("stream response: %v", requestErr)
	case <-time.After(5 * time.Second):
		t.Fatal("stream response was not released")
	}
}

type webSessionResponse struct {
	Authenticated bool   `json:"authenticated"`
	CSRFToken     string `json:"csrfToken"`
}

func newCookieClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	jftradeCheckTestError(t, err)
	return &http.Client{Jar: jar}
}

func requestWebLogin(t *testing.T, client *http.Client, baseURL string, password string) *http.Response {
	t.Helper()
	return requestWebLoginWithOrigin(t, client, baseURL, password, baseURL)
}

func requestWebLoginWithOrigin(t *testing.T, client *http.Client, baseURL string, password string, requestOrigin string) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]string{"password": password})
	jftradeCheckTestError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/api/v1/auth/login", bytes.NewReader(body))
	jftradeCheckTestError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if requestOrigin != "" {
		req.Header.Set("Origin", requestOrigin)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Web login: %v", err)
	}
	return resp
}

func loginWeb(t *testing.T, client *http.Client, baseURL string, password string) string {
	t.Helper()
	resp := requestWebLogin(t, client, baseURL, password)
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	var envelope struct {
		Data webSessionResponse `json:"data"`
	}
	jftradeCheckTestError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	if !envelope.Data.Authenticated || envelope.Data.CSRFToken == "" {
		t.Fatalf("login response = %#v", envelope.Data)
	}
	return envelope.Data.CSRFToken
}
