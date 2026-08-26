package ownerlock

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriterLeaseConflictCrashReleaseAndPersistentDiagnostic(t *testing.T) {
	target := filepath.Join(t.TempDir(), "settings.json")
	command := exec.Command(os.Args[0], "-test.run=TestWriterLeaseHelperProcess")
	command.Env = append(os.Environ(),
		"JFTRADE_OWNER_LOCK_HELPER=hold",
		"JFTRADE_OWNER_LOCK_TARGET="+target,
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("open helper stdout: %v", err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "locked" {
		_ = command.Process.Kill()
		t.Fatalf("helper readiness = %q, err=%v", scanner.Text(), scanner.Err())
	}
	if lease, err := Acquire(target, CurrentDiagnostic("go-test", "conflict")); lease != nil || !errors.Is(err, ErrHeld) {
		t.Fatalf("conflicting acquire = (%v, %v)", lease, err)
	}
	var diagnostic Diagnostic
	if err := decodeDiagnostic(LockPath(target), &diagnostic); err != nil {
		t.Fatalf("decode held diagnostic: %v", err)
	}
	if diagnostic.Owner != "go-helper" || diagnostic.PID != command.Process.Pid || diagnostic.Profile != "crash-test" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if diagnostic.Start <= 0 {
		t.Fatalf("diagnostic start = %d", diagnostic.Start)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("kill helper: %v", err)
	}
	_ = command.Wait()
	lease, err := Acquire(target, CurrentDiagnostic("go-test", "recovered"))
	if err != nil {
		t.Fatalf("acquire after crash: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("release recovered lease: %v", err)
	}
	if _, err := os.Stat(LockPath(target)); err != nil {
		t.Fatalf("lock file must survive release: %v", err)
	}
}

func TestWriterLeaseValidationDefaultsAndIdempotentClose(t *testing.T) {
	t.Setenv("JFTRADE_RUST_REHEARSAL_PROFILE", "")
	diagnostic := CurrentDiagnostic("", "")
	if diagnostic.Owner != "go" || diagnostic.Profile != "go-production" || diagnostic.PID != os.Getpid() {
		t.Fatalf("default diagnostic = %#v", diagnostic)
	}
	t.Setenv("JFTRADE_RUST_REHEARSAL_PROFILE", "rehearsal-test")
	diagnostic = CurrentDiagnostic("  rust-test  ", "")
	if diagnostic.Owner != "rust-test" || diagnostic.Profile != "rehearsal-test" {
		t.Fatalf("environment diagnostic = %#v", diagnostic)
	}
	if lease, err := Acquire("  ", diagnostic); lease != nil || err == nil {
		t.Fatalf("empty target acquire = (%v, %v)", lease, err)
	}
	missingParent := filepath.Join(t.TempDir(), "missing", "settings.json")
	if lease, err := Acquire(missingParent, diagnostic); lease != nil || err == nil {
		t.Fatalf("missing parent acquire = (%v, %v)", lease, err)
	}
	target := filepath.Join(t.TempDir(), "settings.json")
	lease, err := Acquire(target, Diagnostic{})
	if err != nil {
		t.Fatalf("acquire default diagnostic: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("idempotent close: %v", err)
	}
	var nilLease *Lease
	if err := nilLease.Close(); err != nil {
		t.Fatalf("nil close: %v", err)
	}
	if err := unlockFile(nil); err != nil {
		t.Fatalf("nil unlock: %v", err)
	}
	if LockPath("sample.db") != "sample.db.jftrade-owner.lock" {
		t.Fatalf("unexpected LockPath: %q", LockPath("sample.db"))
	}
}

func TestWriterLeaseDiagnosticRejectsClosedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diagnostic.lock")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create diagnostic target: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close diagnostic target: %v", err)
	}
	if err := writeDiagnostic(file, Diagnostic{Owner: "go-test"}); err == nil {
		t.Fatal("writeDiagnostic on closed file succeeded")
	}
	appendOnly, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open append-only diagnostic target: %v", err)
	}
	defer func() { _ = appendOnly.Close() }()
	if err := writeDiagnostic(appendOnly, Diagnostic{Owner: "go-test"}); err == nil {
		t.Fatal("writeDiagnostic with append-only file succeeded")
	}
}

func TestWriterLeaseHelperProcess(t *testing.T) {
	if os.Getenv("JFTRADE_OWNER_LOCK_HELPER") != "hold" {
		return
	}
	target := os.Getenv("JFTRADE_OWNER_LOCK_TARGET")
	lease, err := Acquire(target, CurrentDiagnostic("go-helper", "crash-test"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "acquire helper lease: %v\n", err)
		os.Exit(2)
	}
	defer func() { _ = lease.Close() }()
	fmt.Println("locked")
	for {
		time.Sleep(time.Second)
	}
}

func decodeDiagnostic(path string, destination any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	return decoder.Decode(destination)
}
