package servercore

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/pkg/observability"
)

const requestIDHeader = "X-Request-ID"

func requestObservabilityMiddleware(recorder *observability.Recorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		requestID := normalizeRequestID(c.GetHeader(requestIDHeader))
		if requestID == "" {
			requestID = newRequestID()
		}
		c.Set("requestID", requestID)
		c.Writer.Header().Set(requestIDHeader, requestID)
		requestContext := observability.WithFields(c.Request.Context(), observability.Fields{
			RequestID: requestID,
			Source:    "api",
		})
		requestContext = observability.WithRecorder(requestContext, recorder)
		c.Request = c.Request.WithContext(requestContext)

		c.Next()

		path := c.FullPath()
		if path == "" && c.Request != nil && c.Request.URL != nil {
			path = c.Request.URL.Path
		}
		latency := time.Since(startedAt)
		attrs := []any{
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"latency_ms", latency.Milliseconds(),
			"client_ip", c.ClientIP(),
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, "errors", c.Errors.String())
		}
		observability.InfoWithImportance(requestContext, observability.ImportanceLow, "api request", attrs...)
		if recorder != nil {
			recorder.RecordHTTPRequest(requestContext, c.Request.Method, path, c.Writer.Status(), latency)
		}
	}
}

func normalizeRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			continue
		}
		switch char {
		case '-', '_', '.', ':':
			continue
		default:
			return ""
		}
	}
	return value
}

func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return hex.EncodeToString(buf[:])
	}
	return strings.ReplaceAll(time.Now().UTC().Format(time.RFC3339Nano), ":", "")
}
