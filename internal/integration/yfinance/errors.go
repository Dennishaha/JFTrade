package yfinance

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

var (
	// ErrUnsupported identifies a capability that Yahoo Finance cannot supply.
	ErrUnsupported = marketdata.ErrCapabilityUnsupported
	// ErrSidecarUnavailable identifies transport and server-side sidecar failures.
	ErrSidecarUnavailable = errors.New("yfinance sidecar is unavailable")
	// ErrInvalidResponse identifies a response that violates the sidecar contract.
	ErrInvalidResponse = errors.New("invalid yfinance sidecar response")
)

// HTTPError preserves the structured error returned by the sidecar.
type HTTPError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *HTTPError) Error() string {
	if e == nil {
		return ""
	}
	code := strings.TrimSpace(e.Code)
	message := strings.TrimSpace(e.Message)
	switch {
	case code != "" && message != "":
		return fmt.Sprintf("yfinance sidecar returned HTTP %d (%s): %s", e.StatusCode, code, message)
	case message != "":
		return fmt.Sprintf("yfinance sidecar returned HTTP %d: %s", e.StatusCode, message)
	default:
		return fmt.Sprintf("yfinance sidecar returned HTTP %d", e.StatusCode)
	}
}

// Unwrap classifies server failures as sidecar availability failures while
// keeping 4xx input/not-found errors inspectable as HTTPError.
func (e *HTTPError) Unwrap() error {
	if e != nil && e.StatusCode >= http.StatusInternalServerError {
		return ErrSidecarUnavailable
	}
	return nil
}

func isNotFound(err error) bool {
	var remoteErr *HTTPError
	return errors.As(err, &remoteErr) && remoteErr.StatusCode == http.StatusNotFound
}
