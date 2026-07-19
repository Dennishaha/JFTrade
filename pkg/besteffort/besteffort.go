// Package besteffort provides helpers for best-effort operations whose
// errors should be logged and swallowed instead of failing the caller.
package besteffort

import "log"

// LogError logs a non-nil error. Callers must pass an error explicitly so
// accidental non-error values cannot be silently ignored by this boundary.
// It is intended only where continuing after the failure is deliberate.
func LogError(err error) {
	if err != nil {
		log.Printf("best-effort operation failed: %v", err)
	}
}

// LogResult is the two-result counterpart for cleanup-style APIs such as
// io.StringWriter.WriteString, where the primary value is intentionally ignored.
func LogResult[T any](_ T, err error) {
	LogError(err)
}
