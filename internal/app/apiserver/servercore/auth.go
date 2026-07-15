package servercore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	"github.com/jftrade/jftrade-main/internal/api/middleware"
	"github.com/jftrade/jftrade-main/internal/api/origin"
	"github.com/jftrade/jftrade-main/internal/security/passwordhash"
)

const (
	webSessionCookie = "jftrade_web_session"
	webSessionTTL    = 12 * time.Hour
	loginWindow      = 5 * time.Minute
	maxLoginFailures = 8
	maxLoginAttempts = 1024
	maxWebSessions   = 128
)

var _ middleware.OriginChecker = (*webAuth)(nil)
var _ middleware.Authenticator = (*webAuth)(nil)
var _ middleware.CSRFValidator = (*webAuth)(nil)

var webPasswordCheckSlots = make(chan struct{}, 2)

var errWebAuthConfigurationChanged = errors.New("web access settings changed during login")

type webSession struct {
	CSRF      string
	ExpiresAt time.Time
}

type webLoginRequest struct {
	Password string `json:"password"`
}

type webLoginConfig struct {
	enabled        bool
	unavailable    bool
	passwordHash   string
	generation     uint64
	verifyPassword func(string, string) (bool, error)
}

type loginAttempt struct {
	Failures    int
	WindowStart time.Time
}

// webAuth owns browser password sessions. The Wails WebView uses a separate,
// ephemeral desktop capability and never needs this password.
type webAuth struct {
	enabled             bool
	publicAccessEnabled bool
	webPort             int
	unavailable         bool
	passwordHash        string
	enforceAccess       bool
	generation          uint64
	allowedOrigins      map[string]struct{}
	mu                  sync.Mutex
	sessions            map[string]webSession
	attempts            map[string]loginAttempt
	now                 func() time.Time
	verifyPassword      func(encoded string, password string) (bool, error)
	generateSecret      func(int) (string, error)
	accessContext       context.Context
	cancelAccess        context.CancelFunc
}

func newWebAuth(settings SecuritySettings) *webAuth {
	accessContext, cancelAccess := context.WithCancel(context.Background())
	auth := &webAuth{
		allowedOrigins: map[string]struct{}{},
		sessions:       map[string]webSession{},
		attempts:       map[string]loginAttempt{},
		now:            time.Now,
		verifyPassword: passwordhash.Verify,
		generateSecret: randomSecret,
		accessContext:  accessContext,
		cancelAccess:   cancelAccess,
	}
	auth.configure(settings)
	return auth
}

func (a *webAuth) configure(settings SecuritySettings) {
	if a == nil {
		return
	}
	normalized := normalizeSecuritySettings(settings)
	unavailable := false
	if normalized.WebAccessEnabled {
		if !passwordhash.Valid(normalized.PasswordHash) {
			unavailable = true
		}
	}

	a.mu.Lock()
	changed := a.enabled != normalized.WebAccessEnabled ||
		a.publicAccessEnabled != normalized.PublicAccessEnabled ||
		a.webPort != normalized.WebPort ||
		a.passwordHash != normalized.PasswordHash || a.unavailable != unavailable
	a.enabled = normalized.WebAccessEnabled
	a.publicAccessEnabled = normalized.PublicAccessEnabled
	a.webPort = normalized.WebPort
	a.passwordHash = normalized.PasswordHash
	a.unavailable = unavailable
	if changed {
		a.generation++
		if a.cancelAccess != nil {
			a.cancelAccess()
		}
		a.accessContext, a.cancelAccess = context.WithCancel(context.Background())
		a.sessions = map[string]webSession{}
		a.attempts = map[string]loginAttempt{}
	}
	a.mu.Unlock()
}

func (a *webAuth) currentAccessContext() context.Context {
	if a == nil {
		return context.Background()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.accessContext == nil {
		return context.Background()
	}
	return a.accessContext
}

func (a *webAuth) close() {
	if a == nil {
		return
	}
	a.mu.Lock()
	if a.cancelAccess != nil {
		a.cancelAccess()
	}
	a.sessions = map[string]webSession{}
	a.attempts = map[string]loginAttempt{}
	a.mu.Unlock()
}

func (a *webAuth) configureOrigins(values ...string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, value := range values {
		if configuredOrigin := canonicalOrigin(value); configuredOrigin != "" {
			a.allowedOrigins[configuredOrigin] = struct{}{}
		}
	}
}

func (a *webAuth) originAllowedForRequest(r *http.Request, value string) bool {
	if a == nil {
		return false
	}
	value = canonicalOrigin(value)
	if value == "" {
		return false
	}
	if sameRequestOrigin(r, value) {
		return true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.allowedOrigins[value]
	return ok
}

// IsOriginAllowed implements middleware.OriginChecker.
func (a *webAuth) IsOriginAllowed(r *http.Request, value string) bool {
	return a.originAllowedForRequest(r, value)
}

func canonicalOrigin(value string) string {
	return origin.Canonical(value)
}

func sameRequestOrigin(r *http.Request, value string) bool {
	if r == nil || strings.TrimSpace(r.Host) == "" {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return false
	}
	expectedScheme := requestScheme(r)
	return strings.EqualFold(parsed.Scheme, expectedScheme) && strings.EqualFold(parsed.Host, r.Host)
}

func requestScheme(r *http.Request) string {
	if r != nil && r.TLS != nil {
		return "https"
	}
	// Only a reverse proxy on the same computer may describe the original
	// transport. A network client cannot spoof this header to weaken origin or
	// cookie decisions.
	if remote := requestRemoteIP(r); remote != nil && remote.IsLoopback() {
		forwarded := lastForwardedValue(r.Header.Get("X-Forwarded-Proto"))
		if strings.EqualFold(forwarded, "https") {
			return "https"
		}
	}
	return "http"
}

func requestOrigin(r *http.Request) string {
	if r == nil {
		return ""
	}
	if value := strings.TrimSpace(r.Header.Get("Origin")); value != "" {
		return canonicalOrigin(value)
	}
	return canonicalOrigin(r.Header.Get("Referer"))
}

func requestOriginProvided(r *http.Request) bool {
	if r == nil {
		return false
	}
	return strings.TrimSpace(r.Header.Get("Origin")) != "" || strings.TrimSpace(r.Header.Get("Referer")) != ""
}

func (a *webAuth) requestOriginAllowed(r *http.Request) bool {
	if !requestOriginProvided(r) {
		return true
	}
	value := requestOrigin(r)
	return value != "" && a != nil && a.originAllowedForRequest(r, value)
}

func requestRemoteIP(r *http.Request) net.IP {
	if r == nil {
		return nil
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	return net.ParseIP(strings.Trim(host, "[]"))
}

func requestClientIP(r *http.Request) net.IP {
	peer := requestRemoteIP(r)
	if peer == nil || !peer.IsLoopback() || r == nil {
		return peer
	}
	// A same-host reverse proxy may preserve the original client address. This
	// keeps the public-access gate and login throttling meaningful behind that
	// proxy, while network peers cannot forge X-Forwarded-For.
	forwarded := lastForwardedValue(r.Header.Get("X-Forwarded-For"))
	if parsed := net.ParseIP(strings.Trim(forwarded, "[]")); parsed != nil {
		return parsed
	}
	return peer
}

func lastForwardedValue(header string) string {
	values := strings.Split(header, ",")
	for i := len(values) - 1; i >= 0; i-- {
		if value := strings.TrimSpace(values[i]); value != "" {
			return value
		}
	}
	return ""
}

func (a *webAuth) browserAccessAllowed(r *http.Request) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	enabled := a.enabled
	publicAccessEnabled := a.publicAccessEnabled
	a.mu.Unlock()
	if !enabled {
		return false
	}
	if publicAccessEnabled {
		return true
	}
	remote := requestClientIP(r)
	return remote != nil && remote.IsLoopback()
}

func (a *webAuth) webAccessEnabled() bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.enabled
}

func (a *webAuth) authenticate(r *http.Request) (webSession, bool, bool) {
	if isDesktopRequest(r) {
		return webSession{}, true, true
	}
	if a == nil {
		return webSession{}, false, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.enabled {
		if !a.enforceAccess {
			return webSession{}, true, true
		}
		return webSession{}, false, false
	}
	if a.unavailable {
		return webSession{}, false, false
	}
	cookie, err := r.Cookie(webSessionCookie)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return webSession{}, false, false
	}
	session, ok := a.sessions[cookie.Value]
	if !ok || !session.ExpiresAt.After(a.now()) {
		delete(a.sessions, cookie.Value)
		return webSession{}, false, false
	}
	return session, true, false
}

// Authenticate implements middleware.Authenticator. The third return value is
// true only for the trusted desktop capability (or an explicit test bypass).
func (a *webAuth) Authenticate(r *http.Request) (string, bool, bool) {
	session, ok, trusted := a.authenticate(r)
	return session.CSRF, ok, trusted
}

// ValidateCSRF implements middleware.CSRFValidator.
func (a *webAuth) ValidateCSRF(r *http.Request, session string) bool {
	token := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
	return token != "" && constantTimeEqual(token, session)
}

func bearerToken(header string) string {
	parts := strings.Fields(strings.TrimSpace(header))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func constantTimeEqual(left string, right string) bool {
	leftHash := sha256.Sum256([]byte(left))
	rightHash := sha256.Sum256([]byte(right))
	return subtle.ConstantTimeCompare(leftHash[:], rightHash[:]) == 1
}

// login godoc
// @Summary 登录 JFTrade Web
// @Description 使用用户在桌面设置中配置的 Web 访问密码签发 HttpOnly、SameSite=Strict 会话。
// @Tags auth
// @Accept json
// @Produce json
// @Param request body webLoginRequest true "Web 访问密码"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Failure 401 {object} envelope
// @Failure 429 {object} envelope
// @Router /api/v1/auth/login [post]
func (a *webAuth) login(c *gin.Context) {
	r := c.Request
	if !a.requestOriginAllowed(r) {
		writeAuthError(c, http.StatusForbidden, "ORIGIN_FORBIDDEN", "request origin is not allowed")
		return
	}
	if isDesktopRequest(r) {
		writeAuthJSON(c, http.StatusOK, map[string]any{"authenticated": true, "csrfToken": ""})
		return
	}
	if a == nil {
		writeAuthError(c, http.StatusForbidden, "WEB_ACCESS_DISABLED", "Web access is disabled; enable it in the desktop settings")
		return
	}
	config := a.loginConfig()
	if !config.enabled {
		writeAuthError(c, http.StatusForbidden, "WEB_ACCESS_DISABLED", "Web access is disabled; enable it in the desktop settings")
		return
	}
	if config.unavailable {
		writeAuthError(c, http.StatusServiceUnavailable, "WEB_AUTH_UNAVAILABLE", "Web password authentication is unavailable")
		return
	}
	remote := requestClientIP(r)
	remoteKey := "unknown"
	if remote != nil {
		remoteKey = remote.String()
	}
	if retryAfter, limited := a.loginRateLimited(remoteKey); limited {
		c.Header("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())+1))
		writeAuthError(c, http.StatusTooManyRequests, "LOGIN_RATE_LIMITED", "too many failed login attempts")
		return
	}
	var payload webLoginRequest
	r.Body = http.MaxBytesReader(c.Writer, r.Body, 8<<10)
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeAuthError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid login payload")
		return
	}
	if config.verifyPassword == nil {
		config.verifyPassword = passwordhash.Verify
	}
	passwordMatches, verifyErr := a.checkPasswordForLogin(r.Context(), config.passwordHash, payload.Password, config.verifyPassword)
	if verifyErr != nil && r.Context().Err() != nil {
		writeAuthError(c, http.StatusRequestTimeout, "REQUEST_CANCELED", "login request was canceled")
		return
	}
	if verifyErr != nil || !passwordMatches {
		a.recordLoginFailure(remoteKey)
		writeAuthError(c, http.StatusUnauthorized, "INVALID_PASSWORD", "invalid Web access password")
		return
	}
	a.clearLoginFailures(remoteKey)
	sessionID, session, err := a.createSession(config.generation, config.passwordHash)
	if errors.Is(err, errWebAuthConfigurationChanged) {
		writeAuthError(c, http.StatusConflict, "WEB_AUTH_CONFIGURATION_CHANGED", "Web access settings changed during login; try again")
		return
	}
	if err != nil {
		writeAuthError(c, http.StatusInternalServerError, "WEB_AUTH_FAILED", "failed to create session")
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     webSessionCookie,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestScheme(r) == "https",
		SameSite: http.SameSiteStrictMode,
		Expires:  session.ExpiresAt,
		MaxAge:   int(webSessionTTL.Seconds()),
	})
	writeAuthJSON(c, http.StatusOK, map[string]any{
		"authenticated": true,
		"csrfToken":     session.CSRF,
		"expiresAt":     session.ExpiresAt.UTC().Format(time.RFC3339Nano),
	})
}

func (a *webAuth) loginConfig() webLoginConfig {
	a.mu.Lock()
	defer a.mu.Unlock()
	return webLoginConfig{
		enabled:        a.enabled,
		unavailable:    a.unavailable,
		passwordHash:   a.passwordHash,
		generation:     a.generation,
		verifyPassword: a.verifyPassword,
	}
}

func (a *webAuth) checkPasswordForLogin(ctx context.Context, encoded string, password string, verify func(string, string) (bool, error)) (bool, error) {
	select {
	case webPasswordCheckSlots <- struct{}{}:
		defer func() { <-webPasswordCheckSlots }()
	case <-ctx.Done():
		return false, ctx.Err()
	}
	return verify(encoded, password)
}

func (a *webAuth) createSession(generation uint64, passwordHash string) (string, webSession, error) {
	generateSecret := a.generateSecret
	if generateSecret == nil {
		generateSecret = randomSecret
	}
	sessionID, err := generateSecret(32)
	if err != nil {
		return "", webSession{}, err
	}
	csrf, err := generateSecret(24)
	if err != nil {
		return "", webSession{}, err
	}
	session := webSession{CSRF: csrf, ExpiresAt: a.now().Add(webSessionTTL)}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.generation != generation || !a.enabled || a.unavailable || a.passwordHash != passwordHash {
		return "", webSession{}, errWebAuthConfigurationChanged
	}
	a.pruneSessionsLocked(a.now())
	if len(a.sessions) >= maxWebSessions {
		a.evictOldestSessionLocked()
	}
	a.sessions[sessionID] = session
	return sessionID, session, nil
}

// logout godoc
// @Summary 注销 JFTrade Web 会话
// @Tags auth
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/auth/logout [post]
func (a *webAuth) logout(c *gin.Context) {
	r := c.Request
	if cookie, err := r.Cookie(webSessionCookie); err == nil {
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     webSessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   requestScheme(r) == "https",
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
	writeAuthJSON(c, http.StatusOK, map[string]any{"authenticated": false})
}

// status godoc
// @Summary 读取 JFTrade Web 会话
// @Tags auth
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/auth/session [get]
func (a *webAuth) status(c *gin.Context) {
	if !a.requestOriginAllowed(c.Request) {
		writeAuthError(c, http.StatusForbidden, "ORIGIN_FORBIDDEN", "request origin is not allowed")
		return
	}
	session, ok, trusted := a.authenticate(c.Request)
	data := map[string]any{"authenticated": ok}
	if ok && !trusted {
		data["csrfToken"] = session.CSRF
		data["expiresAt"] = session.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	writeAuthJSON(c, http.StatusOK, data)
}

func (a *webAuth) loginRateLimited(remote string) (time.Duration, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	attempt := a.attempts[remote]
	now := a.now()
	if attempt.WindowStart.IsZero() || now.Sub(attempt.WindowStart) >= loginWindow {
		delete(a.attempts, remote)
		return 0, false
	}
	if attempt.Failures < maxLoginFailures {
		return 0, false
	}
	return loginWindow - now.Sub(attempt.WindowStart), true
}

func (a *webAuth) recordLoginFailure(remote string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	a.pruneLoginAttemptsLocked(now)
	if _, exists := a.attempts[remote]; !exists && len(a.attempts) >= maxLoginAttempts {
		a.evictOldestLoginAttemptLocked()
	}
	attempt := a.attempts[remote]
	if attempt.WindowStart.IsZero() || now.Sub(attempt.WindowStart) >= loginWindow {
		attempt = loginAttempt{WindowStart: now}
	}
	attempt.Failures++
	a.attempts[remote] = attempt
}

func (a *webAuth) pruneLoginAttemptsLocked(now time.Time) {
	for remote, attempt := range a.attempts {
		if attempt.WindowStart.IsZero() || now.Sub(attempt.WindowStart) >= loginWindow {
			delete(a.attempts, remote)
		}
	}
}

func (a *webAuth) evictOldestLoginAttemptLocked() {
	oldestKey := ""
	var oldest time.Time
	for remote, attempt := range a.attempts {
		if oldestKey == "" || attempt.WindowStart.Before(oldest) {
			oldestKey = remote
			oldest = attempt.WindowStart
		}
	}
	if oldestKey != "" {
		delete(a.attempts, oldestKey)
	}
}

func (a *webAuth) pruneSessionsLocked(now time.Time) {
	for id, session := range a.sessions {
		if !session.ExpiresAt.After(now) {
			delete(a.sessions, id)
		}
	}
}

func (a *webAuth) evictOldestSessionLocked() {
	oldestID := ""
	var oldest time.Time
	for id, session := range a.sessions {
		if oldestID == "" || session.ExpiresAt.Before(oldest) {
			oldestID = id
			oldest = session.ExpiresAt
		}
	}
	if oldestID != "" {
		delete(a.sessions, oldestID)
	}
}

func (a *webAuth) clearLoginFailures(remote string) {
	a.mu.Lock()
	delete(a.attempts, remote)
	a.mu.Unlock()
}

func randomSecret(size int) (string, error) {
	return randomSecretFrom(rand.Reader, size)
}

func randomSecretFrom(reader io.Reader, size int) (string, error) {
	raw := make([]byte, size)
	if _, err := io.ReadFull(reader, raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func writeAuthJSON(c *gin.Context, status int, data any) {
	c.Header("Cache-Control", "no-store")
	c.JSON(status, httpserver.Envelope{
		OK:        status >= 200 && status < 300,
		Data:      data,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func writeAuthError(c *gin.Context, status int, code string, message string) {
	c.Header("Cache-Control", "no-store")
	c.AbortWithStatusJSON(status, httpserver.Envelope{
		OK:        false,
		Error:     &httpserver.APIError{Code: code, Message: message},
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
}
