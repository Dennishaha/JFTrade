package adk

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/assistant/engine/providers"
	"github.com/jftrade/jftrade-main/internal/assistant/engine/skillsruntime"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

func httpFetchTool(ctx context.Context, input map[string]any) (any, error) {
	rawURL := jftradeOptionalTypeAssertion[string](input["url"])
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("only http and https are supported")
	}
	if err := rejectUnsafeHost(ctx, parsed.Hostname()); err != nil {
		return nil, err
	}
	timeout := 12 * time.Second
	client := providers.NewSafeHTTPClient(timeout, rejectUnsafeHost)
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := rejectUnsafeHost(req.Context(), req.URL.Hostname()); err != nil {
			return fmt.Errorf("redirect to unsafe host %q blocked: %w", req.URL.Hostname(), err)
		}
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects (max 5)")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "JFTrade-ADK/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { besteffort.LogError(resp.Body.Close()) }()
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if contentType != "" && (!strings.Contains(contentType, "text/") && !strings.Contains(contentType, "json") && !strings.Contains(contentType, "xml") && !strings.Contains(contentType, "rss")) {
		return nil, fmt.Errorf("unsupported content type %q", contentType)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"url":         parsed.String(),
		"status":      resp.StatusCode,
		"contentType": resp.Header.Get("Content-Type"),
		"body":        string(body),
		"truncated":   len(body) >= 1<<20,
		"fetchedAt":   nowString(),
	}, nil
}

const maxWorkflowWaitDuration = 25 * time.Second

func workflowWaitTool(ctx context.Context, input map[string]any) (any, error) {
	duration, err := workflowWaitDuration(input)
	if err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(skillsruntime.StringValue(input, "reason"))
	started := time.Now().UTC()
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}
	completed := time.Now().UTC()
	return map[string]any{
		"waitedMs":    completed.Sub(started).Milliseconds(),
		"startedAt":   started.Format(time.RFC3339Nano),
		"completedAt": completed.Format(time.RFC3339Nano),
		"reason":      reason,
	}, nil
}

func workflowWaitDuration(input map[string]any) (time.Duration, error) {
	durationMs := skillsruntime.IntValue(input, "durationMs", 0)
	if durationMs <= 0 {
		switch value := input["seconds"].(type) {
		case float64:
			durationMs = int(value * 1000)
		case int:
			durationMs = value * 1000
		case string:
			seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err == nil {
				durationMs = int(seconds * 1000)
			}
		}
	}
	if durationMs <= 0 {
		return 0, fmt.Errorf("seconds or durationMs must be greater than 0")
	}
	duration := time.Duration(durationMs) * time.Millisecond
	if duration > maxWorkflowWaitDuration {
		return 0, fmt.Errorf("workflow.wait duration must be <= %s", maxWorkflowWaitDuration)
	}
	return duration, nil
}

func rejectUnsafeHost(ctx context.Context, host string) error {
	return providers.RejectUnsafeHost(ctx, host)
}
