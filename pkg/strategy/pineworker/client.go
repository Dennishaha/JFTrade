package pineworker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Transport interface {
	RunScript(context.Context, RunScriptRequest) (RunScriptResponse, error)
}

type Client struct {
	transport Transport
	config    WorkerConfig
	gate      PerformanceGate
	now       func() time.Time
}

type ClientOption func(*Client)

func NewClient(transport Transport, config WorkerConfig, options ...ClientOption) (*Client, error) {
	if transport == nil {
		return nil, fmt.Errorf("pine worker transport is required")
	}
	client := &Client{
		transport: transport,
		config:    config,
		now:       time.Now,
	}
	if client.config.RequestTimeout <= 0 {
		client.config.RequestTimeout = DefaultWorkerConfig(1).RequestTimeout
	}
	for _, option := range options {
		option(client)
	}
	if client.now == nil {
		client.now = time.Now
	}
	return client, nil
}

func WithPerformanceGate(gate PerformanceGate) ClientOption {
	return func(client *Client) {
		client.gate = gate
	}
}

func WithNow(now func() time.Time) ClientOption {
	return func(client *Client) {
		client.now = now
	}
}

func (client *Client) RunScript(ctx context.Context, request RunScriptRequest) (RunScriptResponse, error) {
	if client == nil {
		return RunScriptResponse{}, fmt.Errorf("pine worker client is nil")
	}
	requestBytes, err := validateAndMeasureRunScriptRequest(request, client.config)
	if err != nil {
		return RunScriptResponse{}, err
	}
	callCtx, cancel := context.WithTimeout(ctx, client.config.RequestTimeout)
	defer cancel()

	started := client.now()
	response, err := client.transport.RunScript(callCtx, request)
	if err != nil {
		return RunScriptResponse{}, mapTransportError(err)
	}
	if response.Error != "" {
		return response, fmt.Errorf("pine worker returned error: %s", response.Error)
	}
	if response.JobID != "" && response.JobID != request.JobID {
		return response, fmt.Errorf("pine worker response job id %q does not match request %q", response.JobID, request.JobID)
	}
	if response.JobID == "" {
		response.JobID = request.JobID
	}

	duration := response.Metadata.Duration
	if duration <= 0 {
		duration = client.now().Sub(started)
		response.Metadata.Duration = duration
	}
	if response.Metadata.RequestBytes <= 0 {
		response.Metadata.RequestBytes = requestBytes
	}
	responseBytes, err := jsonSize(response)
	if err != nil {
		return RunScriptResponse{}, err
	}
	if response.Metadata.ResponseBytes <= 0 {
		response.Metadata.ResponseBytes = responseBytes
	}
	if client.gate != (PerformanceGate{}) {
		if err := CheckPerformanceGate(PerformanceSample{
			Candles:       max(1, len(request.Candles)),
			Duration:      duration,
			RequestBytes:  response.Metadata.RequestBytes,
			ResponseBytes: response.Metadata.ResponseBytes,
			PeakRSSBytes:  response.Metadata.PeakRSSBytes,
		}, client.gate); err != nil {
			return response, fmt.Errorf("pine worker performance gate failed: %w", err)
		}
	}
	return response, nil
}

func mapTransportError(err error) error {
	if err == nil {
		return nil
	}
	if errorsIsDeadline(err) {
		return fmt.Errorf("pine worker request timed out: %w", err)
	}
	return fmt.Errorf("pine worker transport error: %w", err)
}

func errorsIsDeadline(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "deadline exceeded")
}
