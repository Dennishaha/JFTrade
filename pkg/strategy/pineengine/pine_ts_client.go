package pineengine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

type PinetsWorkerClient struct {
	NodePath   string
	WorkerPath string

	mu      sync.Mutex
	counter uint64
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	reader  *bufio.Reader
	stderr  strings.Builder
}

func NewPinetsWorkerClient(nodePath string, workerPath string) *PinetsWorkerClient {
	if strings.TrimSpace(nodePath) == "" {
		nodePath = "node"
	}
	if strings.TrimSpace(workerPath) == "" {
		workerPath = DefaultWorkerPath()
	}
	return &PinetsWorkerClient{NodePath: nodePath, WorkerPath: workerPath}
}

func DefaultWorkerPath() string {
	if value := strings.TrimSpace(os.Getenv("JFTRADE_PINETS_WORKER_PATH")); value != "" {
		return value
	}
	if _, err := os.Stat(filepath.Join("scripts", "pinets-worker.mjs")); err == nil {
		return filepath.Join("scripts", "pinets-worker.mjs")
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("scripts", "pinets-worker.mjs")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "scripts", "pinets-worker.mjs"))
}

func (c *PinetsWorkerClient) EngineInfo(ctx context.Context) (EngineInfo, error) {
	var info EngineInfo
	if err := c.call(ctx, "engineInfo", nil, &info); err != nil {
		return EngineInfo{}, err
	}
	return info, nil
}

func (c *PinetsWorkerClient) RunIndicator(ctx context.Context, req RunIndicatorRequest) (RunIndicatorResponse, error) {
	var out RunIndicatorResponse
	if err := c.call(ctx, "runIndicator", req, &out); err != nil {
		return RunIndicatorResponse{}, err
	}
	return out, nil
}

func (c *PinetsWorkerClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}

func (c *PinetsWorkerClient) call(ctx context.Context, method string, params any, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureStartedLocked(ctx); err != nil {
		return err
	}
	id := atomic.AddUint64(&c.counter, 1)
	request := workerRequest{ID: id, Method: method, Params: params}
	encoded, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if _, err := c.stdin.Write(append(encoded, '\n')); err != nil {
		_ = c.closeLocked()
		return fmt.Errorf("write pinets worker request: %w", err)
	}
	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		_ = c.closeLocked()
		return fmt.Errorf("read pinets worker response: %w%s", err, c.stderrSuffix())
	}
	var response workerResponse
	if err := json.Unmarshal(line, &response); err != nil {
		return fmt.Errorf("decode pinets worker response: %w", err)
	}
	if response.ID != id {
		_ = c.closeLocked()
		return fmt.Errorf("pinets worker response id = %d, want %d", response.ID, id)
	}
	if !response.OK {
		if response.Error.Message == "" {
			response.Error.Message = "pinets worker failed"
		}
		return response.Error
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(response.Result, result); err != nil {
		return fmt.Errorf("decode pinets worker result: %w", err)
	}
	return nil
}

func (c *PinetsWorkerClient) ensureStartedLocked(ctx context.Context) error {
	if c.cmd != nil && c.stdin != nil && c.reader != nil {
		return nil
	}
	if _, err := os.Stat(c.WorkerPath); err != nil {
		return fmt.Errorf("pinets worker script unavailable at %s: %w", c.WorkerPath, err)
	}
	cmd := exec.CommandContext(ctx, c.NodePath, c.WorkerPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start pinets worker: %w", err)
	}
	c.cmd = cmd
	c.stdin = stdin
	c.reader = bufio.NewReader(stdout)
	c.stderr.Reset()
	go c.captureStderr(stderr)
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

func (c *PinetsWorkerClient) captureStderr(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		c.mu.Lock()
		if c.stderr.Len() < 4096 {
			c.stderr.WriteString(scanner.Text())
			c.stderr.WriteByte('\n')
		}
		c.mu.Unlock()
	}
}

func (c *PinetsWorkerClient) closeLocked() error {
	var joined error
	if c.stdin != nil {
		joined = errors.Join(joined, c.stdin.Close())
	}
	if c.cmd != nil && c.cmd.Process != nil {
		joined = errors.Join(joined, c.cmd.Process.Kill())
	}
	c.cmd = nil
	c.stdin = nil
	c.reader = nil
	return joined
}

func (c *PinetsWorkerClient) stderrSuffix() string {
	if c.stderr.Len() == 0 {
		return ""
	}
	return ": " + strings.TrimSpace(c.stderr.String())
}

type workerRequest struct {
	ID     uint64 `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type workerResponse struct {
	ID     uint64          `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  workerError     `json:"error"`
}

type workerError struct {
	Code        string       `json:"code"`
	Message     string       `json:"message"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func (e workerError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func ExternalEnginePayloadFromResult(mode string, result RunIndicatorResponse, err error) ExternalEnginePayload {
	payload := DisabledPayload()
	payload.Enabled = mode != ModeOff
	payload.Mode = mode
	payload.Status = "shadow_error"
	payload.OK = false
	payload.DifferenceSummary = map[string]any{"evaluated": false}
	if err != nil {
		payload.Diagnostics = []Diagnostic{{
			Severity:  "error",
			Code:      "PINETS_SHADOW_ERROR",
			Message:   err.Error(),
			Line:      1,
			Column:    1,
			EndLine:   1,
			EndColumn: 1,
		}}
		return payload
	}
	payload.EngineVersion = result.EngineVersion
	payload.License = result.License
	payload.OK = result.OK
	payload.Status = "shadow_ok"
	payload.Diagnostics = result.Diagnostics
	payload.DifferenceSummary = map[string]any{
		"evaluated": true,
		"plots":     len(result.Plots),
		"signals":   len(result.Signals),
		"authority": "pine-pinets production runtime remains authoritative",
	}
	return payload
}
