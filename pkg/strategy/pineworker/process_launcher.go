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
	"strings"
	"time"
)

const nodeWorkerMaxOldSpaceMiB = 384

type WorkerBundle struct {
	Name   string
	Data   []byte
	SHA256 string
}

type NodeWorkerLauncherConfig struct {
	Bundle          WorkerBundle
	RuntimePath     string
	TempDir         string
	WorkDir         string
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

type NodeWorkerLauncher struct {
	config NodeWorkerLauncherConfig
}

func NewNodeWorkerLauncher(config NodeWorkerLauncherConfig) (*NodeWorkerLauncher, error) {
	if len(config.Bundle.Data) == 0 {
		return nil, fmt.Errorf("pine worker bundle data is required")
	}
	if config.Bundle.Name == "" {
		config.Bundle.Name = "worker.mjs"
	}
	if strings.TrimSpace(config.RuntimePath) == "" {
		config.RuntimePath = "node"
	}
	if config.StopTimeout <= 0 {
		config.StopTimeout = 5 * time.Second
	}
	if config.MaxMessageBytes <= 0 {
		config.MaxMessageBytes = DefaultWorkerConfig(1).MaxMessageBytes
	}
	return &NodeWorkerLauncher{config: config}, nil
}

func (launcher *NodeWorkerLauncher) Start(ctx context.Context, spec WorkerSpec) (WorkerProcess, error) {
	if launcher == nil {
		return nil, fmt.Errorf("pine worker launcher is nil")
	}
	path, err := launcher.materializeBundle(spec)
	if err != nil {
		return nil, err
	}
	args := launcher.args(spec)
	if err := ctx.Err(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	commandPath := strings.TrimSpace(launcher.config.RuntimePath)
	commandArgs := append([]string{path}, args...)
	cmd := exec.Command(commandPath, commandArgs...)
	if strings.TrimSpace(launcher.config.WorkDir) != "" {
		cmd.Dir = launcher.config.WorkDir
	}
	cmd.Env = nodeWorkerEnvironment(launcher.config.Env)
	cmd.Stdout = launcher.config.Stdout
	cmd.Stderr = launcher.config.Stderr
	if err := cmd.Start(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("start pine worker process: %w", err)
	}
	return &OSWorkerProcess{
		cmd:         cmd,
		path:        path,
		runtimePath: strings.TrimSpace(launcher.config.RuntimePath),
		args:        append([]string(nil), commandArgs...),
		workDir:     cmd.Dir,
		stdout:      launcher.config.Stdout,
		stderr:      launcher.config.Stderr,
		stopTimeout: launcher.config.StopTimeout,
	}, nil
}

func nodeWorkerEnvironment(extra []string) []string {
	input := append(os.Environ(), extra...)
	environment := make([]string, 0, len(input)+1)
	nodeOptions := ""
	for _, entry := range input {
		key, value, found := strings.Cut(entry, "=")
		if found && strings.EqualFold(key, "NODE_OPTIONS") {
			nodeOptions = value
			continue
		}
		environment = append(environment, entry)
	}
	fields := strings.Fields(nodeOptions)
	filtered := fields[:0]
	for index := 0; index < len(fields); index++ {
		field := fields[index]
		if field == "--max-old-space-size" || field == "--max_old_space_size" {
			index++
			continue
		}
		if strings.HasPrefix(field, "--max-old-space-size=") || strings.HasPrefix(field, "--max_old_space_size=") {
			continue
		}
		filtered = append(filtered, field)
	}
	filtered = append(filtered, fmt.Sprintf("--max-old-space-size=%d", nodeWorkerMaxOldSpaceMiB))
	return append(environment, "NODE_OPTIONS="+strings.Join(filtered, " "))
}

func (launcher *NodeWorkerLauncher) materializeBundle(spec WorkerSpec) (string, error) {
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
	path := filepath.Join(dir, fmt.Sprintf("%s-%s", spec.WorkerID, launcher.config.Bundle.Name))
	if err := os.WriteFile(path, launcher.config.Bundle.Data, 0o644); err != nil {
		return "", fmt.Errorf("write pine worker bundle: %w", err)
	}
	return path, nil
}

func (launcher *NodeWorkerLauncher) verifyChecksum() error {
	expected := launcher.config.Bundle.SHA256
	if expected == "" {
		return nil
	}
	sum := sha256.Sum256(launcher.config.Bundle.Data)
	actual := hex.EncodeToString(sum[:])
	if actual != expected {
		return fmt.Errorf("pine worker bundle checksum mismatch: %s != %s", actual, expected)
	}
	return nil
}

func (launcher *NodeWorkerLauncher) args(spec WorkerSpec) []string {
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
	runtimePath string
	args        []string
	workDir     string
	stdout      io.Writer
	stderr      io.Writer
	stopTimeout time.Duration
}

func (process *OSWorkerProcess) Diagnostics() string {
	if process == nil {
		return ""
	}
	parts := []string{}
	if process.path != "" {
		parts = append(parts, "bundle="+process.path)
	}
	if process.runtimePath != "" {
		parts = append(parts, "runtime="+process.runtimePath)
	}
	if process.workDir != "" {
		parts = append(parts, "cwd="+process.workDir)
	}
	if len(process.args) > 0 {
		parts = append(parts, "args="+strings.Join(process.args, " "))
	}
	if stdout := writerString(process.stdout); stdout != "" {
		parts = append(parts, "stdout="+stdout)
	}
	if stderr := writerString(process.stderr); stderr != "" {
		parts = append(parts, "stderr="+stderr)
	}
	return strings.Join(parts, "; ")
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

func writerString(writer io.Writer) string {
	stringer, ok := writer.(interface{ String() string })
	if !ok {
		return ""
	}
	return summarizeProcessLog(stringer.String())
}

func summarizeProcessLog(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 2000 {
		return value
	}
	return value[len(value)-2000:]
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
