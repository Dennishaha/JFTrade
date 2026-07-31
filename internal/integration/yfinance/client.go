package yfinance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

const (
	defaultMaxAttempts = 3
	defaultRetryDelay  = 100 * time.Millisecond
	maxRetryDelay      = time.Second
	maxResponseBytes   = 4 << 20
)

type Client struct {
	baseURL     *url.URL
	httpClient  *http.Client
	maxAttempts int
	retryDelay  time.Duration
}

// NewClient creates a sidecar client. Callers may supply a custom HTTP client
// for tests or transport instrumentation.
func NewClient(baseURL string, httpClient *http.Client) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid yfinance sidecar URL %q", baseURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("invalid yfinance sidecar URL scheme %q", parsed.Scheme)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultRequestTimeout}
	}
	return &Client{
		baseURL:     parsed,
		httpClient:  httpClient,
		maxAttempts: defaultMaxAttempts,
		retryDelay:  defaultRetryDelay,
	}, nil
}

func (c *Client) get(ctx context.Context, segments []string, query url.Values, target any) error {
	if c == nil || c.baseURL == nil || c.httpClient == nil {
		return ErrSidecarUnavailable
	}
	callCtx := ctx
	cancel := func() {}
	if c.httpClient.Timeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, c.httpClient.Timeout)
	}
	defer cancel()
	endpoint := c.baseURL.JoinPath(segments...)
	endpoint.RawQuery = query.Encode()
	attempts := max(c.maxAttempts, 1)
	for attempt := 1; attempt <= attempts; attempt++ {
		body, status, header, err := c.getOnce(callCtx, endpoint.String())
		if err == nil && status >= http.StatusOK && status < http.StatusMultipleChoices {
			return decodeResponse(body, target)
		}
		if err != nil {
			if ctxErr := callCtx.Err(); ctxErr != nil {
				return ctxErr
			}
			if errors.Is(err, ErrInvalidResponse) {
				return err
			}
			if attempt == attempts {
				return fmt.Errorf("%w: GET %s: %w", ErrSidecarUnavailable, endpoint.Redacted(), err)
			}
		} else if !isRetryableStatus(status) || attempt == attempts {
			return decodeHTTPError(status, body)
		}
		if err := waitForRetry(callCtx, retryWait(header, c.retryDelay, attempt)); err != nil {
			return err
		}
	}
	return ErrSidecarUnavailable
}

func (c *Client) getOnce(ctx context.Context, endpoint string) ([]byte, int, http.Header, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, 0, nil, err
	}
	defer func() { besteffort.LogError(response.Body.Close()) }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, response.StatusCode, response.Header, err
	}
	if len(body) > maxResponseBytes {
		return nil, response.StatusCode, response.Header, fmt.Errorf("%w: response exceeds %d bytes", ErrInvalidResponse, maxResponseBytes)
	}
	return body, response.StatusCode, response.Header, nil
}

func decodeResponse(body []byte, target any) error {
	if len(bytes.TrimSpace(body)) == 0 {
		return fmt.Errorf("%w: empty response body", ErrInvalidResponse)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidResponse, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("%w: trailing JSON content", ErrInvalidResponse)
	}
	return nil
}

func decodeHTTPError(status int, body []byte) error {
	var envelope remoteErrorEnvelope
	if err := decodeResponse(body, &envelope); err == nil && envelope.Error.Message != "" {
		return &HTTPError{StatusCode: status, Code: envelope.Error.Code, Message: envelope.Error.Message}
	}
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(status)
	}
	if len(message) > 512 {
		message = message[:512]
	}
	return &HTTPError{StatusCode: status, Message: message}
}

func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests:
		return true
	default:
		return status >= http.StatusInternalServerError
	}
}

func retryWait(header http.Header, base time.Duration, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header.Get("Retry-After"))); err == nil && seconds >= 0 {
		return min(time.Duration(seconds)*time.Second, maxRetryDelay)
	}
	return min(time.Duration(attempt)*max(base, 0), maxRetryDelay)
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) health(ctx context.Context) (remoteHealth, error) {
	var response remoteHealth
	if err := c.get(ctx, []string{"health"}, nil, &response); err != nil {
		return remoteHealth{}, err
	}
	if strings.TrimSpace(response.YFinanceVersion) == "" {
		return remoteHealth{}, fmt.Errorf(
			"%w: health response field yfinance_version is required",
			ErrInvalidResponse,
		)
	}
	return response, nil
}

func (c *Client) markets(ctx context.Context) ([]remoteMarketProfile, error) {
	var response remoteMarkets
	err := c.get(ctx, []string{"markets"}, nil, &response)
	return response.Markets, err
}

func (c *Client) search(ctx context.Context, query string, limit int) ([]remoteInstrument, error) {
	values := url.Values{}
	values.Set("q", strings.TrimSpace(query))
	values.Set("limit", strconv.Itoa(limit))
	var response remoteSearch
	err := c.get(ctx, []string{"search"}, values, &response)
	return response.Entries, err
}

func (c *Client) security(ctx context.Context, market, symbol string) (remoteSecurity, error) {
	var response remoteSecurity
	err := c.get(ctx, []string{"security", market, symbol}, nil, &response)
	return response, err
}

func (c *Client) snapshot(ctx context.Context, market, symbol string) (remoteSnapshot, error) {
	var response remoteSnapshot
	err := c.get(ctx, []string{"snapshot", market, symbol}, nil, &response)
	return response, err
}

func (c *Client) candles(
	ctx context.Context,
	market string,
	symbol string,
	period string,
	limit int,
	fromTime string,
	toTime string,
) (remoteCandles, error) {
	values := url.Values{}
	values.Set("period", period)
	values.Set("limit", strconv.Itoa(limit))
	if value := strings.TrimSpace(fromTime); value != "" {
		values.Set("from", value)
	}
	if value := strings.TrimSpace(toTime); value != "" {
		values.Set("to", value)
	}
	var response remoteCandles
	err := c.get(ctx, []string{"candles", market, symbol}, values, &response)
	return response, err
}
