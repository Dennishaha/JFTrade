// Package ownerlock provides cross-process exclusive writer leases for
// settings files and SQLite databases during the Go-to-Rust migration.
package ownerlock

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const Suffix = ".jftrade-owner.lock"

var (
	ErrHeld          = errors.New("writer lease is already held")
	processStartedAt = time.Now().UTC()
)

type Diagnostic struct {
	Owner   string `json:"owner"`
	PID     int    `json:"pid"`
	Start   int64  `json:"start"`
	Profile string `json:"profile"`
}

func CurrentDiagnostic(owner string, profile string) Diagnostic {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		owner = "go"
	}
	profile = strings.TrimSpace(profile)
	if profile == "" {
		profile = strings.TrimSpace(os.Getenv("JFTRADE_RUST_REHEARSAL_PROFILE"))
	}
	if profile == "" {
		profile = "go-production"
	}
	return Diagnostic{
		Owner: owner, PID: os.Getpid(),
		Start: processStartedAt.UnixMilli(), Profile: profile,
	}
}

type Lease struct {
	file      *os.File
	closeOnce sync.Once
	closeErr  error
}

func Acquire(target string, diagnostic Diagnostic) (*Lease, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("writer lease target path is required")
	}
	path := LockPath(target)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open writer lease %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure writer lease %s: %w", path, err)
	}
	if err := tryLockFile(file); err != nil {
		_ = file.Close()
		if isLockConflict(err) {
			return nil, fmt.Errorf("%w for %s", ErrHeld, target)
		}
		return nil, fmt.Errorf("lock writer lease %s: %w", path, err)
	}
	if err := writeDiagnostic(file, diagnostic); err != nil {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, fmt.Errorf("write writer lease diagnostic %s: %w", path, err)
	}
	return &Lease{file: file}, nil
}

func LockPath(target string) string { return strings.TrimSpace(target) + Suffix }

func (l *Lease) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.closeOnce.Do(func() {
		l.closeErr = errors.Join(unlockFile(l.file), l.file.Close())
	})
	return l.closeErr
}

func writeDiagnostic(file *os.File, diagnostic Diagnostic) error {
	if strings.TrimSpace(diagnostic.Owner) == "" {
		diagnostic = CurrentDiagnostic("go", diagnostic.Profile)
	}
	encoded, err := json.Marshal(diagnostic)
	if err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if _, err := file.WriteAt(encoded, 0); err != nil {
		return err
	}
	return file.Sync()
}
