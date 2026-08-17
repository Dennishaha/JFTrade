package akshare

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

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

const (
	defaultRequestTimeout = 15 * time.Second
	defaultMaxAttempts    = 3
	defaultRetryDelay     = 100 * time.Millisecond
	maxRetryDelay         = time.Second
	maxResponseBytes      = 8 << 20
)

var providerPath = []string{"providers", "akshare"}

type Client struct {
	baseURL     *url.URL
	httpClient  *http.Client
	maxAttempts int
	retryDelay  time.Duration
}

// NewClient creates an AKShare client for the application-owned market-data
// sidecar. The endpoint is the process root, not a provider-specific URL.
func NewClient(baseURL string, httpClient *http.Client) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid market-data sidecar URL %q", baseURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("invalid market-data sidecar URL scheme %q", parsed.Scheme)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultRequestTimeout}
	}
	return &Client{
		baseURL: parsed, httpClient: httpClient,
		maxAttempts: defaultMaxAttempts, retryDelay: defaultRetryDelay,
	}, nil
}

func (c *Client) get(ctx context.Context, segments []string, query url.Values, target any) error {
	return c.request(ctx, http.MethodGet, segments, query, nil, target)
}

func (c *Client) post(ctx context.Context, segments []string, input, target any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode AKShare sidecar request: %w", err)
	}
	return c.request(ctx, http.MethodPost, segments, nil, body, target)
}

func (c *Client) request(
	ctx context.Context,
	method string,
	segments []string,
	query url.Values,
	body []byte,
	target any,
) error {
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
	for attempt := 1; attempt <= max(c.maxAttempts, 1); attempt++ {
		responseBody, status, header, err := c.requestOnce(callCtx, method, endpoint.String(), body)
		if err == nil && status >= http.StatusOK && status < http.StatusMultipleChoices {
			return decodeResponse(responseBody, target)
		}
		if err != nil {
			if contextErr := callCtx.Err(); contextErr != nil {
				return contextErr
			}
			if errors.Is(err, ErrInvalidResponse) {
				return err
			}
			if attempt == max(c.maxAttempts, 1) {
				return fmt.Errorf("%w: %s %s: %w", ErrSidecarUnavailable, method, endpoint.Redacted(), err)
			}
		} else {
			runtimeErr := decodeHTTPError(status, responseBody)
			// Pool saturation and an already timed-out worker are explicit
			// backpressure signals. Retrying immediately only creates another
			// request storm against the same four slots; let the API layer expose
			// Retry-After instead.
			if isProviderBusyError(runtimeErr) ||
				!isRetryableStatus(status) || attempt == max(c.maxAttempts, 1) {
				return classifyRuntimeError(runtimeErr)
			}
		}
		if err := waitForRetry(callCtx, retryWait(header, c.retryDelay, attempt)); err != nil {
			return err
		}
	}
	return ErrSidecarUnavailable
}

func (c *Client) requestOnce(
	ctx context.Context,
	method string,
	endpoint string,
	body []byte,
) ([]byte, int, http.Header, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, nil, err
	}
	request.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, 0, nil, err
	}
	defer func() { besteffort.LogError(response.Body.Close()) }()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, response.StatusCode, response.Header, err
	}
	if len(responseBody) > maxResponseBytes {
		return nil, response.StatusCode, response.Header,
			fmt.Errorf("%w: response exceeds %d bytes", ErrInvalidResponse, maxResponseBytes)
	}
	return responseBody, response.StatusCode, response.Header, nil
}

func classifyRuntimeError(err error) error {
	var remoteErr *HTTPError
	if !errors.As(err, &remoteErr) {
		return err
	}
	switch strings.ToUpper(strings.TrimSpace(remoteErr.Code)) {
	case "AKSHARE_RUNTIME_WARMING", "PROVIDER_RUNTIME_WARMING":
		return fmt.Errorf("%w: %w", marketdata.ErrProviderWarming, remoteErr)
	case "AKSHARE_POOL_BUSY", "AKSHARE_UPSTREAM_TIMEOUT":
		return fmt.Errorf("%w: %w", marketdata.ErrProviderBusy, remoteErr)
	case "UNSUPPORTED_RANGE", "UNSUPPORTED_PERIOD", "UNSUPPORTED_ADJUSTMENT", "AKSHARE_UNSUPPORTED":
		return fmt.Errorf("%w: %w", ErrUnsupported, remoteErr)
	default:
		return err
	}
}

func isProviderBusyError(err error) bool {
	var remoteErr *HTTPError
	if !errors.As(err, &remoteErr) {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(remoteErr.Code)) {
	case "AKSHARE_POOL_BUSY", "AKSHARE_UPSTREAM_TIMEOUT":
		return true
	default:
		return false
	}
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

func providerSegments(values ...string) []string {
	result := append([]string(nil), providerPath...)
	return append(result, values...)
}

func (c *Client) health(ctx context.Context) (remoteHealth, error) {
	var response remoteHealth
	if err := c.get(ctx, providerSegments("health"), nil, &response); err != nil {
		return remoteHealth{}, err
	}
	version := strings.TrimSpace(response.AKShareVersion)
	if version == "" {
		version = strings.TrimSpace(response.ProviderVersion)
	}
	if version == "" {
		version = strings.TrimSpace(response.Version)
	}
	switch response.RuntimeState {
	case "warming", "ready", "failed":
	default:
		return remoteHealth{}, fmt.Errorf("%w: health runtime_state is invalid", ErrInvalidResponse)
	}
	if response.RuntimeState == "ready" && version == "" {
		return remoteHealth{}, fmt.Errorf("%w: health response version is required", ErrInvalidResponse)
	}
	return response, nil
}

func (c *Client) markets(ctx context.Context) ([]remoteMarketProfile, error) {
	var response remoteMarkets
	err := c.get(ctx, providerSegments("markets"), nil, &response)
	return response.Markets, err
}

func (c *Client) search(ctx context.Context, query string, limit int) ([]remoteInstrument, error) {
	values := url.Values{"q": {strings.TrimSpace(query)}, "limit": {strconv.Itoa(limit)}}
	var response remoteSearch
	err := c.get(ctx, providerSegments("search"), values, &response)
	return response.Entries, err
}

func (c *Client) security(ctx context.Context, marketValue, symbol string) (remoteSecurity, error) {
	var response remoteSecurity
	err := c.get(ctx, providerSegments("security", marketValue, symbol), nil, &response)
	return response, err
}

func (c *Client) snapshot(ctx context.Context, marketValue, symbol string) (remoteSnapshot, error) {
	var response remoteSnapshot
	err := c.get(ctx, providerSegments("snapshot", marketValue, symbol), nil, &response)
	return response, err
}

func (c *Client) snapshots(ctx context.Context, ids []string) (remoteBatchSnapshots, error) {
	var response remoteBatchSnapshots
	err := c.post(ctx, providerSegments("snapshots"), remoteBatchRequest{InstrumentIDs: ids}, &response)
	return response, err
}

func (c *Client) candles(
	ctx context.Context,
	marketValue string,
	symbol string,
	period string,
	adjustment string,
	limit int,
	fromTime string,
	toTime string,
	beforeTime string,
	sessionSets ...[]string,
) (remoteCandles, error) {
	values := url.Values{"period": {period}, "limit": {strconv.Itoa(limit)}}
	if value := strings.ToLower(strings.TrimSpace(adjustment)); value != "" && value != "none" {
		values.Set("adjustment", value)
	}
	if value := strings.TrimSpace(fromTime); value != "" {
		values.Set("from", value)
	}
	if value := strings.TrimSpace(toTime); value != "" {
		values.Set("to", value)
	}
	if value := strings.TrimSpace(beforeTime); value != "" {
		values.Set("before", value)
	}
	if len(sessionSets) > 0 && len(sessionSets[0]) > 0 {
		values.Set("sessions", strings.Join(sessionSets[0], ","))
	}
	var response remoteCandles
	err := c.get(ctx, providerSegments("candles", marketValue, symbol), values, &response)
	return response, err
}

func (c *Client) news(ctx context.Context, marketValue, symbol string, limit int) (remoteNews, error) {
	values := url.Values{"limit": {strconv.Itoa(limit)}}
	var response remoteNews
	err := c.get(ctx, providerSegments("news", marketValue, symbol), values, &response)
	return response, err
}

func (c *Client) indexConstituents(
	ctx context.Context,
	marketValue string,
	symbol string,
	limit int,
) (remoteIndexConstituents, error) {
	values := url.Values{"limit": {strconv.Itoa(limit)}}
	var response remoteIndexConstituents
	err := c.get(ctx, providerSegments("index-constituents", marketValue, symbol), values, &response)
	return response, err
}

func (c *Client) corporateActions(
	ctx context.Context,
	marketValue string,
	symbol string,
	from time.Time,
	to time.Time,
) (remoteCorporateActions, error) {
	values := url.Values{}
	if !from.IsZero() {
		values.Set("from", from.UTC().Format(time.RFC3339))
	}
	if !to.IsZero() {
		values.Set("to", to.UTC().Format(time.RFC3339))
	}
	var response remoteCorporateActions
	err := c.get(ctx, providerSegments("corporate-actions", marketValue, symbol), values, &response)
	return response, err
}

func (c *Client) rankings(
	ctx context.Context,
	marketValue string,
	kind string,
	limit int,
) (remoteRankings, error) {
	values := url.Values{
		"market": {marketValue}, "kind": {kind}, "limit": {strconv.Itoa(limit)},
	}
	var response remoteRankings
	err := c.get(ctx, providerSegments("rankings"), values, &response)
	return response, err
}

func (c *Client) industries(ctx context.Context, kind string) (remoteIndustryBoards, error) {
	values := url.Values{"kind": {kind}}
	var response remoteIndustryBoards
	err := c.get(ctx, providerSegments("industries"), values, &response)
	return response, err
}

func (c *Client) industryMembers(
	ctx context.Context,
	kind string,
	board string,
	limit int,
) (remoteIndustryMembers, error) {
	values := url.Values{"limit": {strconv.Itoa(limit)}}
	if value := strings.TrimSpace(kind); value != "" {
		values.Set("kind", value)
	}
	var response remoteIndustryMembers
	err := c.get(ctx, providerSegments("industries", board, "members"), values, &response)
	return response, err
}

func (c *Client) companyProfile(ctx context.Context, marketValue, symbol string) (remoteCompanyProfile, error) {
	var response remoteCompanyProfile
	err := c.get(ctx, providerSegments("profile", marketValue, symbol), nil, &response)
	return response, err
}

func (c *Client) financialStatements(
	ctx context.Context,
	marketValue string,
	symbol string,
	statement string,
) (remoteFinancialStatements, error) {
	values := url.Values{"statement": {statement}}
	var response remoteFinancialStatements
	err := c.get(ctx, providerSegments("financials", marketValue, symbol), values, &response)
	return response, err
}

func (c *Client) ownership(ctx context.Context, marketValue, symbol string) (remoteOwnership, error) {
	var response remoteOwnership
	err := c.get(ctx, providerSegments("ownership", marketValue, symbol), nil, &response)
	return response, err
}

func (c *Client) analystConsensus(ctx context.Context, marketValue, symbol string) (remoteAnalystConsensus, error) {
	var response remoteAnalystConsensus
	err := c.get(ctx, providerSegments("analyst", marketValue, symbol), nil, &response)
	return response, err
}

func (c *Client) earningsCalendar(
	ctx context.Context,
	beginDate, endDate string,
) (remoteEarningsCalendar, error) {
	values := url.Values{}
	values.Set("begin_date", beginDate)
	values.Set("end_date", endDate)
	var response remoteEarningsCalendar
	err := c.get(ctx, providerSegments("calendar", "earnings"), values, &response)
	return response, err
}

func (c *Client) dividendCalendar(ctx context.Context, date string) (remoteDividendCalendar, error) {
	values := url.Values{}
	values.Set("date", date)
	var response remoteDividendCalendar
	err := c.get(ctx, providerSegments("calendar", "dividends"), values, &response)
	return response, err
}

func (c *Client) economicCalendar(
	ctx context.Context,
	beginDate, endDate string,
) (remoteEconomicCalendar, error) {
	values := url.Values{}
	values.Set("begin_date", beginDate)
	values.Set("end_date", endDate)
	var response remoteEconomicCalendar
	err := c.get(ctx, providerSegments("calendar", "economic"), values, &response)
	return response, err
}

func (c *Client) ipoCalendar(ctx context.Context) (remoteIpoCalendar, error) {
	var response remoteIpoCalendar
	err := c.get(ctx, providerSegments("calendar", "ipos"), nil, &response)
	return response, err
}

func (c *Client) macroIndicators(ctx context.Context) (remoteMacroIndicators, error) {
	var response remoteMacroIndicators
	err := c.get(ctx, providerSegments("macro", "indicators"), nil, &response)
	return response, err
}

func (c *Client) macroIndicatorHistory(
	ctx context.Context,
	indicatorID string,
	limit int,
) (remoteMacroIndicatorHistory, error) {
	values := url.Values{}
	values.Set("indicator_id", indicatorID)
	values.Set("limit", strconv.Itoa(limit))
	var response remoteMacroIndicatorHistory
	err := c.get(ctx, providerSegments("macro", "indicator-history"), values, &response)
	return response, err
}

func (c *Client) screen(ctx context.Context, req remoteScreenRequest) (remoteScreenResponse, error) {
	var response remoteScreenResponse
	err := c.post(ctx, providerSegments("screen"), req, &response)
	return response, err
}
