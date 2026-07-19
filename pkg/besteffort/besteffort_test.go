package besteffort

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
)

func TestLogError(t *testing.T) {
	var buf bytes.Buffer
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
	})

	LogError(errors.New("close failed"))

	output := buf.String()
	if !strings.Contains(output, "best-effort operation failed: close failed") {
		t.Fatalf("expected best-effort log line, got %q", output)
	}
	if strings.Count(output, "best-effort operation failed") != 1 {
		t.Fatalf("expected exactly one log line, got %q", output)
	}
}

func TestLogErrorNoError(t *testing.T) {
	var buf bytes.Buffer
	originalWriter := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(originalWriter) })

	LogError(nil)
	LogResult(1, nil)

	if buf.Len() != 0 {
		t.Fatalf("expected no log output, got %q", buf.String())
	}
}
