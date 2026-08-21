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
