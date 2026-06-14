package servercore

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	"github.com/jftrade/jftrade-main/internal/api/middleware"
)

const (
	adminSessionCookie = "jftrade_admin_session"
	adminSessionTTL    = 12 * time.Hour
	loginWindow        = 5 * time.Minute
	maxLoginFailures   = 8
)

var _ middleware.OriginChecker = (*adminAuth)(nil)
var _ middleware.Authenticator = (*adminAuth)(nil)
var _ middleware.CSRFValidator = (*adminAuth)(nil)

type adminSession struct {
	CSRF      string
	ExpiresAt time.Time
}

type adminLoginRequest struct {
	Key string `json:"key"`
}

type loginAttempt struct {
	Failures    int
	WindowStart time.Time
}

type adminAuth struct {
	enabled        bool
	unavailable    bool
	key            string
	keyPath        string
	secureCookies  bool
	allowedOrigins map[string]struct{}
	mu             sync.Mutex
	sessions       map[string]adminSession
	attempts       map[string]loginAttempt
	now            func() time.Time
}

func newAdminAuth(settingsPath string) (*adminAuth, error) {
	keyPath := deriveAdminKeyPath(settingsPath)
	key := strings.TrimSpace(os.Getenv("JFTRADE_ADMIN_KEY"))
	if key == "" {
		raw, err := os.ReadFile(keyPath)
		switch {
		case err == nil:
			key = strings.TrimSpace(string(raw))
		case errors.Is(err, os.ErrNotExist):
			key, err = randomSecret(32)
			if err != nil {
				return nil, err
			}
			if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
				return nil, fmt.Errorf("create admin key directory: %w", err)
			}
			if err := os.WriteFile(keyPath, []byte(key+"\n"), 0o600); err != nil {
				return nil, fmt.Errorf("persist admin key: %w", err)
			}
		default:
			return nil, fmt.Errorf("read admin key: %w", err)
		}
	}
	if len(key) < 32 {
		return nil, fmt.Errorf("JFTrade admin key must contain at least 32 characters")
	}
	return &adminAuth{
		enabled:        true,
		key:            key,
		keyPath:        keyPath,
		allowedOrigins: map[string]struct{}{},
		sessions:       map[string]adminSession{},
		attempts:       map[string]loginAttempt{},
		now:            time.Now,
	}, nil
}

func deriveAdminKeyPath(settingsPath string) string {
	if path := strings.TrimSpace(os.Getenv("JFTRADE_ADMIN_KEY_FILE")); path != "" {
		return path
	}
	dir := filepath.Dir(strings.TrimSpace(settingsPath))
	if dir == "" || dir == "." {
		return filepath.Join("secrets", "admin.key")
	}
	return filepath.Join(dir, "secrets", "admin.key")
}

func (a *adminAuth) configureOrigins(values ...string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, value := range values {
		if origin := canonicalOrigin(value); origin != "" {
			a.allowedOrigins[origin] = struct{}{}
		}
	}
}

func (a *adminAuth) originAllowed(origin string) bool {
	if a == nil {
		return false
	}
	origin = canonicalOrigin(origin)
	if origin == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.allowedOrigins[origin]
	return ok
}

// IsOriginAllowed 实现 middleware.OriginChecker 接口。
func (a *adminAuth) IsOriginAllowed(origin string) bool {
	return a.originAllowed(origin)
}

func canonicalOrigin(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

func requestOrigin(r *http.Request) string {
	if r == nil {
		return ""
	}
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		return canonicalOrigin(origin)
	}
	if referer := strings.TrimSpace(r.Header.Get("Referer")); referer != "" {
		return canonicalOrigin(referer)
	}
	return ""
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

func (a *adminAuth) authenticate(r *http.Request) (adminSession, bool, bool) {
	if a == nil || !a.enabled {
		return adminSession{}, true, true
	}
	if a.unavailable {
		return adminSession{}, false, false
	}
	if bearer := bearerToken(r.Header.Get("Authorization")); bearer != "" && constantTimeEqual(bearer, a.key) {
		return adminSession{}, true, true
	}
	cookie, err := r.Cookie(adminSessionCookie)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return adminSession{}, false, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	session, ok := a.sessions[cookie.Value]
	if !ok || !session.ExpiresAt.After(a.now()) {
		delete(a.sessions, cookie.Value)
		return adminSession{}, false, false
	}
	return session, true, false
}

// Authenticate 实现 middleware.Authenticator 接口。
func (a *adminAuth) Authenticate(r *http.Request) (string, bool, bool) {
	session, ok, bearer := a.authenticate(r)
	return session.CSRF, ok, bearer
}

// ValidateCSRF 实现 middleware.CSRFValidator 接口。
func (a *adminAuth) ValidateCSRF(r *http.Request, session string) bool {
	token := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
	if token == "" {
		return false
	}
	return constantTimeEqual(token, session)
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
// @Summary 管理员登录
// @Description 使用单管理员密钥签发 HttpOnly、SameSite=Strict 会话。
// @Tags auth
// @Accept json
// @Produce json
// @Param request body adminLoginRequest true "管理员密钥"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Failure 401 {object} envelope
// @Failure 429 {object} envelope
// @Router /api/v1/auth/login [post]
func (a *adminAuth) login(c *gin.Context) {
	r := c.Request
	if a == nil || !a.enabled {
		writeAuthJSON(c, http.StatusOK, map[string]any{"authenticated": true, "csrfToken": ""})
		return
	}
	if a.unavailable {
		writeAuthError(c, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "administrator authentication is unavailable")
		return
	}
	if origin := requestOrigin(r); origin != "" && !a.originAllowed(origin) {
		writeAuthError(c, http.StatusForbidden, "ORIGIN_FORBIDDEN", "request origin is not allowed")
		return
	}
	remote := requestRemoteIP(r)
	remoteKey := "unknown"
	if remote != nil {
		remoteKey = remote.String()
	}
	if retryAfter, limited := a.loginRateLimited(remoteKey); limited {
		c.Header("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())+1))
		writeAuthError(c, http.StatusTooManyRequests, "LOGIN_RATE_LIMITED", "too many failed login attempts")
		return
	}
	var payload adminLoginRequest
	r.Body = http.MaxBytesReader(c.Writer, r.Body, 8<<10)
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeAuthError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid login payload")
		return
	}
	if !constantTimeEqual(strings.TrimSpace(payload.Key), a.key) {
		a.recordLoginFailure(remoteKey)
		writeAuthError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid administrator key")
		return
	}
	a.clearLoginFailures(remoteKey)
	sessionID, err := randomSecret(32)
	if err != nil {
		writeAuthError(c, http.StatusInternalServerError, "AUTH_FAILED", "failed to create session")
		return
	}
	csrf, err := randomSecret(24)
	if err != nil {
		writeAuthError(c, http.StatusInternalServerError, "AUTH_FAILED", "failed to create session")
		return
	}
	expires := a.now().Add(adminSessionTTL)
	a.mu.Lock()
	a.sessions[sessionID] = adminSession{CSRF: csrf, ExpiresAt: expires}
	a.mu.Unlock()
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     adminSessionCookie,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteStrictMode,
		Expires:  expires,
		MaxAge:   int(adminSessionTTL.Seconds()),
	})
	writeAuthJSON(c, http.StatusOK, map[string]any{
		"authenticated": true,
		"csrfToken":     csrf,
		"expiresAt":     expires.UTC().Format(time.RFC3339Nano),
	})
}

// logout godoc
// @Summary 管理员注销
// @Description 注销当前管理员会话。
// @Tags auth
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/auth/logout [post]
func (a *adminAuth) logout(c *gin.Context) {
	r := c.Request
	if cookie, err := r.Cookie(adminSessionCookie); err == nil {
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     adminSessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   a != nil && a.secureCookies,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
	writeAuthJSON(c, http.StatusOK, map[string]any{"authenticated": false})
}

// status godoc
// @Summary 读取管理员会话
// @Tags auth
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/auth/session [get]
func (a *adminAuth) status(c *gin.Context) {
	session, ok, bearer := a.authenticate(c.Request)
	data := map[string]any{"authenticated": ok}
	if ok && !bearer {
		data["csrfToken"] = session.CSRF
		data["expiresAt"] = session.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	writeAuthJSON(c, http.StatusOK, data)
}

func (a *adminAuth) loginRateLimited(remote string) (time.Duration, bool) {
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

func (a *adminAuth) recordLoginFailure(remote string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	attempt := a.attempts[remote]
	if attempt.WindowStart.IsZero() || now.Sub(attempt.WindowStart) >= loginWindow {
		attempt = loginAttempt{WindowStart: now}
	}
	attempt.Failures++
	a.attempts[remote] = attempt
}

func (a *adminAuth) clearLoginFailures(remote string) {
	a.mu.Lock()
	delete(a.attempts, remote)
	a.mu.Unlock()
}

func randomSecret(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func writeAuthJSON(c *gin.Context, status int, data any) {
	c.JSON(status, httpserver.Envelope{
		OK:        status >= 200 && status < 300,
		Data:      data,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func writeAuthError(c *gin.Context, status int, code string, message string) {
	c.AbortWithStatusJSON(status, httpserver.Envelope{
		OK:        false,
		Error:     &httpserver.APIError{Code: code, Message: message},
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
}
