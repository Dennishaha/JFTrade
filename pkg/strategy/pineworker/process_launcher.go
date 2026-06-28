package pineworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

type WorkerBinary struct {
	Name   string
	Data   []byte
	SHA256 string
}

type BinaryWorkerLauncherConfig struct {
	Binary          WorkerBinary
	TempDir         string
	ProtoPath       string
	MaxMessageBytes int
	PineTSVersion   string
	Mock            bool
	ExtraArgs       []string
	Env             []string
	Stdout          io.Writer
	Stderr          io.Writer
	StopTimeout     time.Duration
}

type BinaryWorkerLauncher struct {
	config BinaryWorkerLauncherConfig
}

func NewBinaryWorkerLauncher(config BinaryWorkerLauncherConfig) (*BinaryWorkerLauncher, error) {
	if len(config.Binary.Data) == 0 {
		return nil, fmt.Errorf("pine worker binary data is required")
	}
	if config.Binary.Name == "" {
		config.Binary.Name = "pineworker"
	}
	if config.StopTimeout <= 0 {
		config.StopTimeout = 5 * time.Second
	}
	if config.MaxMessageBytes <= 0 {
		config.MaxMessageBytes = DefaultWorkerConfig(1).MaxMessageBytes
	}
	return &BinaryWorkerLauncher{config: config}, nil
}

func (launcher *BinaryWorkerLauncher) Start(ctx context.Context, spec WorkerSpec) (WorkerProcess, error) {
	if launcher == nil {
		return nil, fmt.Errorf("pine worker launcher is nil")
	}
	path, err := launcher.materializeBinary(spec)
	if err != nil {
		return nil, err
	}
	args := launcher.args(spec)
	cmd := exec.CommandContext(ctx, path, args...)
	if len(launcher.config.Env) > 0 {
		cmd.Env = append(os.Environ(), launcher.config.Env...)
	}
	cmd.Stdout = launcher.config.Stdout
	cmd.Stderr = launcher.config.Stderr
	if err := cmd.Start(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("start pine worker process: %w", err)
	}
	return &OSWorkerProcess{
		cmd:         cmd,
		path:        path,
		stopTimeout: launcher.config.StopTimeout,
	}, nil
}

func (launcher *BinaryWorkerLauncher) materializeBinary(spec WorkerSpec) (string, error) {
	if err := launcher.verifyChecksum(); err != nil {
		return "", err
	}
	dir := launcher.config.TempDir
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "jftrade-pineworker-*")
		if err != nil {
			return "", fmt.Errorf("create pine worker temp dir: %w", err)
		}
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create pine worker temp dir: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s", spec.WorkerID, launcher.config.Binary.Name))
	if err := os.WriteFile(path, launcher.config.Binary.Data, 0o755); err != nil {
		return "", fmt.Errorf("write pine worker binary: %w", err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return "", fmt.Errorf("chmod pine worker binary: %w", err)
	}
	return path, nil
}

func (launcher *BinaryWorkerLauncher) verifyChecksum() error {
	expected := launcher.config.Binary.SHA256
	if expected == "" {
		return nil
	}
	sum := sha256.Sum256(launcher.config.Binary.Data)
	actual := hex.EncodeToString(sum[:])
	if actual != expected {
		return fmt.Errorf("pine worker binary checksum mismatch: %s != %s", actual, expected)
	}
	return nil
}

func (launcher *BinaryWorkerLauncher) args(spec WorkerSpec) []string {
	args := []string{
		"--address", spec.Address,
		"--worker-id", spec.WorkerID,
		"--max-message-bytes", strconv.Itoa(launcher.config.MaxMessageBytes),
	}
	if launcher.config.ProtoPath != "" {
		args = append(args, "--proto", launcher.config.ProtoPath)
	}
	if launcher.config.PineTSVersion != "" {
		args = append(args, "--pinets-version", launcher.config.PineTSVersion)
	}
	if launcher.config.Mock {
		args = append(args, "--mock", "true")
	}
	return append(args, launcher.config.ExtraArgs...)
}

type OSWorkerProcess struct {
	cmd         *exec.Cmd
	path        string
	stopTimeout time.Duration
}

func (process *OSWorkerProcess) Stop(ctx context.Context) error {
	if process == nil || process.cmd == nil || process.cmd.Process == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() {
		done <- process.cmd.Wait()
	}()
	_ = process.cmd.Process.Signal(os.Interrupt)
	select {
	case err := <-done:
		_ = os.Remove(process.path)
		return ignoreProcessExit(err)
	case <-time.After(process.stopTimeout):
		_ = process.cmd.Process.Kill()
	case <-ctx.Done():
		_ = process.cmd.Process.Kill()
	}
	err := <-done
	_ = os.Remove(process.path)
	return ignoreProcessExit(err)
}

func ignoreProcessExit(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		return nil
	}
	return err
}
