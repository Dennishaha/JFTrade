package akshare

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

var (
	ErrUnsupported        = marketdata.ErrCapabilityUnsupported
	ErrSidecarUnavailable = errors.New("AKShare sidecar is unavailable")
	ErrInvalidResponse    = errors.New("invalid AKShare sidecar response")
)

// HTTPError preserves a structured error returned by the Python sidecar.
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
		return fmt.Sprintf("AKShare sidecar returned HTTP %d (%s): %s", e.StatusCode, code, message)
	case message != "":
		return fmt.Sprintf("AKShare sidecar returned HTTP %d: %s", e.StatusCode, message)
	default:
		return fmt.Sprintf("AKShare sidecar returned HTTP %d", e.StatusCode)
	}
}

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
