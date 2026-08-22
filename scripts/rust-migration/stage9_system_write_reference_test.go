package rustmigration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	systemapi "github.com/jftrade/jftrade-main/internal/api/system"
	sysservice "github.com/jftrade/jftrade-main/internal/system"
	trading "github.com/jftrade/jftrade-main/internal/trading"
)

const (
	stage9SystemWriteFixtureVersion = "stage9.system-write.v1"
	stage9SystemWriteTimestamp      = "2026-08-23T10:00:00Z"
)

type stage9SystemWriteFixture struct {
	Version string                         `json:"version"`
	Cases   []stage9SystemWriteFixtureCase `json:"cases"`
}

type stage9SystemWriteFixtureCase struct {
	Name                string              `json:"name"`
	Method              string              `json:"method"`
	RequestPaths        []string            `json:"requestPaths"`
	RequestBodies       []string            `json:"requestBodies"`
	ContextError        string              `json:"contextError,omitempty"`
	ExpectedStatuses    []int               `json:"expectedStatuses"`
	PortCalls           []bool              `json:"portCalls"`
	ResponseHeaders     []map[string]string `json:"responseHeaders"`
	Responses           []json.RawMessage   `json:"responses"`
	ExpectedObservation map[string]any      `json:"expectedObservation"`
}

type stage9SystemWriteCaseSpec struct {
	Name         string
	Method       string
	Paths        []string
	Bodies       []string
	ContextError string
	Setup        func(*testing.T) *stage9SystemWriteHarness
}

type stage9SystemWriteHarness struct {
	router   *gin.Engine
	controls *stage9SystemWriteControls
}

type stage9SystemWriteControls struct {
	resetCalls    []string
	riskUpdates   []sysservice.RealTradeRuntimeRiskCommand
	riskDisables  []sysservice.RealTradeRuntimeRiskCommand
	killActivates []sysservice.RealTradeKillSwitchCommand
	killReleases  []sysservice.RealTradeKillSwitchCommand
	hardActivates []sysservice.RealTradeHardStopCommand
	hardReleases  []stage9SystemWriteHardReleaseCall

	result       trading.RealTradeRiskSnapshot
	honorContext bool
	updateErr    error
	disableErr   error
	killOnErr    error
	killOffErr   error
	hardOnErr    error
	hardOffErr   error
}

type stage9SystemWriteHardReleaseCall struct {
	HardStopID string
	Command    sysservice.RealTradeHardStopCommand
}

func (c *stage9SystemWriteControls) reset() {
	c.resetCalls = append(c.resetCalls, "manual-retry")
}

func (c *stage9SystemWriteControls) updateRisk(
	ctx context.Context,
	command sysservice.RealTradeRuntimeRiskCommand,
) (trading.RealTradeRiskSnapshot, error) {
	c.riskUpdates = append(c.riskUpdates, command)
	if c.honorContext && ctx.Err() != nil {
		return trading.RealTradeRiskSnapshot{}, ctx.Err()
	}
	if c.updateErr != nil {
		return trading.RealTradeRiskSnapshot{}, c.updateErr
	}
	return c.result, nil
}

func (c *stage9SystemWriteControls) disableRisk(
	ctx context.Context,
	command sysservice.RealTradeRuntimeRiskCommand,
) (trading.RealTradeRiskSnapshot, error) {
	c.riskDisables = append(c.riskDisables, command)
	if c.honorContext && ctx.Err() != nil {
		return trading.RealTradeRiskSnapshot{}, ctx.Err()
	}
	if c.disableErr != nil {
		return trading.RealTradeRiskSnapshot{}, c.disableErr
	}
	return c.result, nil
}

func (c *stage9SystemWriteControls) activateKillSwitch(
	ctx context.Context,
	command sysservice.RealTradeKillSwitchCommand,
) (trading.RealTradeRiskSnapshot, error) {
	c.killActivates = append(c.killActivates, command)
	if c.honorContext && ctx.Err() != nil {
		return trading.RealTradeRiskSnapshot{}, ctx.Err()
	}
	if c.killOnErr != nil {
		return trading.RealTradeRiskSnapshot{}, c.killOnErr
	}
	return c.result, nil
}

func (c *stage9SystemWriteControls) releaseKillSwitch(
	ctx context.Context,
	command sysservice.RealTradeKillSwitchCommand,
) (trading.RealTradeRiskSnapshot, error) {
	c.killReleases = append(c.killReleases, command)
	if c.honorContext && ctx.Err() != nil {
		return trading.RealTradeRiskSnapshot{}, ctx.Err()
	}
	if c.killOffErr != nil {
		return trading.RealTradeRiskSnapshot{}, c.killOffErr
	}
	return c.result, nil
}

func (c *stage9SystemWriteControls) activateHardStop(
	ctx context.Context,
	command sysservice.RealTradeHardStopCommand,
) (trading.RealTradeRiskSnapshot, error) {
	c.hardActivates = append(c.hardActivates, command)
	if c.honorContext && ctx.Err() != nil {
		return trading.RealTradeRiskSnapshot{}, ctx.Err()
	}
	if c.hardOnErr != nil {
		return trading.RealTradeRiskSnapshot{}, c.hardOnErr
	}
	return c.result, nil
}

func (c *stage9SystemWriteControls) releaseHardStop(
	ctx context.Context,
	hardStopID string,
	command sysservice.RealTradeHardStopCommand,
) (trading.RealTradeRiskSnapshot, error) {
	c.hardReleases = append(c.hardReleases, stage9SystemWriteHardReleaseCall{
		HardStopID: hardStopID,
		Command:    command,
	})
	if c.honorContext && ctx.Err() != nil {
		return trading.RealTradeRiskSnapshot{}, ctx.Err()
	}
	if c.hardOffErr != nil {
		return trading.RealTradeRiskSnapshot{}, c.hardOffErr
	}
	return c.result, nil
}

func newStage9SystemWriteHarness(
	controls *stage9SystemWriteControls,
) *stage9SystemWriteHarness {
	svc := sysservice.NewService(
		sysservice.WithResetBrokerRuntime(controls.reset),
		sysservice.WithRealTradeRuntimeRiskControls(controls.updateRisk, controls.disableRisk),
		sysservice.WithRealTradeKillSwitchControls(controls.activateKillSwitch, controls.releaseKillSwitch),
		sysservice.WithRealTradeHardStopControls(controls.activateHardStop, controls.releaseHardStop),
	)
	router := gin.New()
	systemapi.RegisterRoutes(router.Group("/api/v1"), svc)
	return &stage9SystemWriteHarness{router: router, controls: controls}
}

func stage9SystemWriteBase(t *testing.T, result string) *stage9SystemWriteHarness {
	t.Helper()
	return newStage9SystemWriteHarness(&stage9SystemWriteControls{
		result: stage9SystemWriteSnapshot(result),
	})
}

func stage9SystemWriteWithContext(t *testing.T, result string) *stage9SystemWriteHarness {
	harness := stage9SystemWriteBase(t, result)
	harness.controls.honorContext = true
	return harness
}

func stage9SystemWriteRequest(
	t *testing.T,
	harness *stage9SystemWriteHarness,
	method, path, body, contextError string,
) (int, map[string]string, map[string]any) {
	t.Helper()
	ctx := context.Background()
	var cancel context.CancelFunc
	switch contextError {
	case "canceled":
		ctx, cancel = context.WithCancel(ctx)
		cancel()
	case "deadline":
		ctx, cancel = context.WithDeadline(ctx, time.Unix(0, 0))
		cancel()
	}
	request := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, request)
	var envelope map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s %s response: %v (%s)", method, path, err, recorder.Body.String())
	}
	headers := make(map[string]string, len(recorder.Header()))
	for key, values := range recorder.Header() {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	return recorder.Code, headers, envelope
}

func stage9RunSystemWriteCase(
	t *testing.T,
	spec stage9SystemWriteCaseSpec,
) stage9SystemWriteFixtureCase {
	t.Helper()
	if len(spec.Paths) != len(spec.Bodies) {
		t.Fatalf("case %s paths/bodies length mismatch", spec.Name)
	}
	harness := spec.Setup(t)
	statuses := make([]int, len(spec.Paths))
	portCalls := make([]bool, len(spec.Paths))
	headers := make([]map[string]string, len(spec.Paths))
	responses := make([]json.RawMessage, len(spec.Paths))
	for index := range spec.Paths {
		status, responseHeaders, envelope := stage9SystemWriteRequest(
			t, harness, spec.Method, spec.Paths[index], spec.Bodies[index], spec.ContextError,
		)
		stage9NormalizeSystemWriteValue(envelope)
		encoded, err := json.Marshal(envelope)
		if err != nil {
			t.Fatalf("encode case %s response: %v", spec.Name, err)
		}
		statuses[index] = status
		headers[index] = responseHeaders
		portCalls[index] = stage9SystemWritePortCall(
			spec.Method, spec.Paths[index], spec.Bodies[index], status,
		)
		responses[index] = encoded
	}
	return stage9SystemWriteFixtureCase{
		Name:                spec.Name,
		Method:              spec.Method,
		RequestPaths:        append([]string(nil), spec.Paths...),
		RequestBodies:       append([]string(nil), spec.Bodies...),
		ContextError:        spec.ContextError,
		ExpectedStatuses:    statuses,
		PortCalls:           portCalls,
		ResponseHeaders:     headers,
		Responses:           responses,
		ExpectedObservation: harness.observation(),
	}
}

func stage9SystemWritePortCall(method, path, body string, status int) bool {
	if status != http.StatusOK {
		return status == http.StatusConflict
	}
	if method == http.MethodPost && path == "/api/v1/system/futu-opend/manual-retry" {
		return true
	}
	if method == http.MethodPost && path == "/api/v1/system/real-trade-kill-switch/release" {
		return optionalSystemWriteBody(body)
	}
	if method == http.MethodPost && strings.Contains(path, "/real-trade-hard-stops/") {
		decoded, err := url.PathUnescape(strings.TrimSuffix(
			strings.TrimPrefix(path, "/api/v1/system/real-trade-hard-stops/"),
			"/release",
		))
		return err == nil && strings.TrimSpace(decoded) != "" && optionalSystemWriteBody(body)
	}
	switch method {
	case http.MethodPost, http.MethodPut:
		return requiredSystemWriteBody(body)
	case http.MethodDelete:
		return optionalSystemWriteBody(body)
	default:
		return false
	}
}

func requiredSystemWriteBody(body string) bool {
	if body == "" {
		return false
	}
	value, ok := stage9FirstJSONValue(body)
	if !ok || value == nil {
		return ok
	}
	_, object := value.(map[string]any)
	return object
}

func optionalSystemWriteBody(body string) bool {
	if body == "" {
		return true
	}
	value, ok := stage9FirstJSONValue(body)
	if !ok || value == nil {
		return ok
	}
	_, object := value.(map[string]any)
	return object
}

func stage9FirstJSONValue(body string) (any, bool) {
	decoder := json.NewDecoder(strings.NewReader(body))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	return value, true
}

func stage9NormalizeSystemWriteValue(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if key == "timestamp" {
				current[key] = stage9SystemWriteTimestamp
				continue
			}
			stage9NormalizeSystemWriteValue(child)
		}
	case []any:
		for _, child := range current {
			stage9NormalizeSystemWriteValue(child)
		}
	}
}

func (h *stage9SystemWriteHarness) observation() map[string]any {
	hardReleases := make([]any, 0, len(h.controls.hardReleases))
	for _, call := range h.controls.hardReleases {
		hardReleases = append(hardReleases, map[string]any{
			"hardStopId": call.HardStopID,
			"command":    stage9SystemWriteJSONValue(call.Command),
		})
	}
	return map[string]any{
		"resetCalls":        append([]string{}, h.controls.resetCalls...),
		"riskUpdateCalls":   stage9SystemWriteCommands(h.controls.riskUpdates),
		"riskDisableCalls":  stage9SystemWriteCommands(h.controls.riskDisables),
		"killActivateCalls": stage9SystemWriteCommands(h.controls.killActivates),
		"killReleaseCalls":  stage9SystemWriteCommands(h.controls.killReleases),
		"hardActivateCalls": stage9SystemWriteCommands(h.controls.hardActivates),
		"hardReleaseCalls":  hardReleases,
	}
}

func stage9SystemWriteCommands(commands any) []any {
	encoded, err := json.Marshal(commands)
	if err != nil {
		panic(err)
	}
	var values []any
	if err := json.Unmarshal(encoded, &values); err != nil {
		panic(err)
	}
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, map[string]any{"command": value})
	}
	return result
}

func stage9SystemWriteJSONValue(value any) any {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		panic(err)
	}
	return decoded
}

func stage9SystemWriteSnapshot(kind string) trading.RealTradeRiskSnapshot {
	result := trading.RealTradeRiskSnapshot{
		ControlPlaneAvailable: true,
		KillSwitchEvents:      []trading.RealTradeControlEvent{},
		BlockedOperations:     []string{"PLACE", "MODIFY"},
		AllowsCancel:          true,
		HardStopEntries:       []trading.RealTradeHardStopEntry{},
		HardStopEvents:        []trading.RealTradeControlEvent{},
		RiskEvents:            []trading.RealTradeControlEvent{},
	}
	switch kind {
	case "risk-update":
		quantity := 12.5
		result.RealTradingEnabled = true
		result.RiskEnabled = true
		result.RuntimeRiskConfigured = true
		result.RuntimeConfiguredMaxOrderQuantity = &quantity
		result.EffectiveMaxOrderQuantity = &quantity
		result.RiskEntry = &trading.RealTradeRuntimeRiskEntry{
			ID: "runtime-risk-config", TradingEnvironment: "REAL", RealTradingEnabled: true,
			MaxOrderQuantity: &quantity, OperatorID: "fixture-operator", Reason: "fixture update",
			ActivatedAt: stage9SystemWriteTimestamp, UpdatedAt: stage9SystemWriteTimestamp,
		}
		result.RiskEvents = []trading.RealTradeControlEvent{
			stage9SystemWriteEvent("risk-event", "updated", "RISK_CONFIG_UPDATED", "*"),
		}
	case "risk-disable":
		result.RiskEvents = []trading.RealTradeControlEvent{
			stage9SystemWriteEvent("risk-disable-event", "disabled", "RISK_CONFIG_DISABLED", "*"),
		}
	case "kill-activate":
		result.RealTradingEnabled = true
		result.KillSwitchActive = true
		result.RuntimeKillSwitchActive = true
		source := "RUNTIME"
		result.KillSwitchSource = &source
		result.KillSwitchEntry = &trading.RealTradeKillSwitchEntry{
			ID: "kill-switch-control-plane", TradingEnvironment: "REAL", OperatorID: "fixture-operator",
			Reason: "fixture incident", ActivatedAt: stage9SystemWriteTimestamp, UpdatedAt: stage9SystemWriteTimestamp,
		}
		result.KillSwitchEvents = []trading.RealTradeControlEvent{
			stage9SystemWriteEvent("kill-event", "activated", "KILL_SWITCH_ACTIVATE", "*"),
		}
	case "kill-release":
		result.RealTradingEnabled = true
		result.KillSwitchEvents = []trading.RealTradeControlEvent{
			stage9SystemWriteEvent("kill-release-event", "released", "KILL_SWITCH_RELEASE", "*"),
		}
	case "hard-activate":
		result.RealTradingEnabled = true
		market := "US"
		symbol := "AAPL"
		entry := trading.RealTradeHardStopEntry{
			ID: "hard-stop-fixture", BrokerID: "futu", TradingEnvironment: "REAL", AccountID: "ACC-1",
			Market: &market, Symbol: &symbol, HardStopScope: "SYMBOL", OperatorID: "fixture-operator",
			Reason: "fixture incident", ActivatedAt: stage9SystemWriteTimestamp, UpdatedAt: stage9SystemWriteTimestamp,
		}
		result.HardStopsActive = true
		result.HardStopEntries = []trading.RealTradeHardStopEntry{entry}
		result.HardStopEvents = []trading.RealTradeControlEvent{
			stage9SystemWriteEvent("hard-stop-event", "activated", "HARD_STOP_ACTIVATE", "futu"),
		}
	case "hard-release":
		result.RealTradingEnabled = true
		result.HardStopEvents = []trading.RealTradeControlEvent{
			stage9SystemWriteEvent("hard-release-event", "released", "HARD_STOP_RELEASE", "futu"),
		}
	}
	return result
}

func stage9SystemWriteEvent(id, eventType, action, brokerID string) trading.RealTradeControlEvent {
	environment := "REAL"
	return trading.RealTradeControlEvent{
		ID: id, EventType: eventType, Action: action, BrokerID: brokerID,
		TradingEnvironment: &environment, CreatedAt: stage9SystemWriteTimestamp,
	}
}

func stage9SystemWriteCaseSpecs() []stage9SystemWriteCaseSpec {
	base := func(kind string) func(*testing.T) *stage9SystemWriteHarness {
		return func(t *testing.T) *stage9SystemWriteHarness { return stage9SystemWriteBase(t, kind) }
	}
	withContext := func(kind string) func(*testing.T) *stage9SystemWriteHarness {
		return func(t *testing.T) *stage9SystemWriteHarness { return stage9SystemWriteWithContext(t, kind) }
	}
	return []stage9SystemWriteCaseSpec{
		{Name: "manual-retry-success", Method: http.MethodPost, Paths: []string{"/api/v1/system/futu-opend/manual-retry"}, Bodies: []string{""}, Setup: base("manual")},
		{Name: "manual-retry-ignores-malformed-and-repeats", Method: http.MethodPost, Paths: []string{"/api/v1/system/futu-opend/manual-retry", "/api/v1/system/futu-opend/manual-retry"}, Bodies: []string{"{", `{"ignored":true}{"trailing":true}`}, Setup: base("manual")},
		{Name: "manual-retry-canceled-still-accepted", Method: http.MethodPost, Paths: []string{"/api/v1/system/futu-opend/manual-retry"}, Bodies: []string{"not-json"}, ContextError: "canceled", Setup: base("manual")},

		{Name: "hard-stop-activate-success-unknown-field", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-hard-stops"}, Bodies: []string{`{"brokerId":" Futu ","tradingEnvironment":" real ","accountId":" ACC-1 ","market":"us","symbol":" aapl ","hardStopScope":"symbol","operatorId":" operator ","reason":" incident ","unknownField":true}`}, Setup: base("hard-activate")},
		{Name: "hard-stop-activate-null-and-trailing", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-hard-stops", "/api/v1/system/real-trade-hard-stops"}, Bodies: []string{"null", `{"accountId":"ACC-1"}{"accountId":"ignored"}`}, Setup: base("hard-activate")},
		{Name: "hard-stop-activate-required-body-errors", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-hard-stops", "/api/v1/system/real-trade-hard-stops", "/api/v1/system/real-trade-hard-stops"}, Bodies: []string{"", "{", "[]"}, Setup: base("hard-activate")},
		{Name: "hard-stop-activate-control-failure", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-hard-stops"}, Bodies: []string{"{}"}, Setup: func(t *testing.T) *stage9SystemWriteHarness {
			h := stage9SystemWriteBase(t, "hard-activate")
			h.controls.hardOnErr = errors.New("real-trade control persistence unavailable")
			return h
		}},
		{Name: "hard-stop-activate-canceled", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-hard-stops"}, Bodies: []string{"{}"}, ContextError: "canceled", Setup: withContext("hard-activate")},
		{Name: "hard-stop-activate-deadline", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-hard-stops"}, Bodies: []string{"{}"}, ContextError: "deadline", Setup: withContext("hard-activate")},
		{Name: "hard-stop-activate-repeated", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-hard-stops", "/api/v1/system/real-trade-hard-stops"}, Bodies: []string{`{"accountId":"ACC-1"}`, `{"accountId":"ACC-1"}`}, Setup: base("hard-activate")},

		{Name: "hard-stop-release-success-trimmed-path", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-hard-stops/%20hs-1%20/release"}, Bodies: []string{""}, Setup: base("hard-release")},
		{Name: "hard-stop-release-null-and-trailing", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-hard-stops/hs-1/release", "/api/v1/system/real-trade-hard-stops/hs-1/release"}, Bodies: []string{"null", `{"operatorId":"operator"}{"reason":"ignored"}`}, Setup: base("hard-release")},
		{Name: "hard-stop-release-blank-id", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-hard-stops/%20/release"}, Bodies: []string{""}, Setup: base("hard-release")},
		{Name: "hard-stop-release-malformed-before-port", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-hard-stops/hs-1/release"}, Bodies: []string{"{"}, Setup: base("hard-release")},
		{Name: "hard-stop-release-control-failure", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-hard-stops/hs-1/release"}, Bodies: []string{"{}"}, Setup: func(t *testing.T) *stage9SystemWriteHarness {
			h := stage9SystemWriteBase(t, "hard-release")
			h.controls.hardOffErr = errors.New("real-trade hard stop not found")
			return h
		}},
		{Name: "hard-stop-release-canceled", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-hard-stops/hs-1/release"}, Bodies: []string{"{}"}, ContextError: "canceled", Setup: withContext("hard-release")},
		{Name: "hard-stop-release-deadline", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-hard-stops/hs-1/release"}, Bodies: []string{"{}"}, ContextError: "deadline", Setup: withContext("hard-release")},
		{Name: "hard-stop-release-repeated", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-hard-stops/hs-1/release", "/api/v1/system/real-trade-hard-stops/hs-1/release"}, Bodies: []string{"", ""}, Setup: base("hard-release")},

		{Name: "kill-activate-success-unknown-field", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-kill-switch/activate"}, Bodies: []string{`{"tradingEnvironment":"REAL","operatorId":"operator","reason":"incident","unknownField":true}`}, Setup: base("kill-activate")},
		{Name: "kill-activate-null-and-trailing", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-kill-switch/activate", "/api/v1/system/real-trade-kill-switch/activate"}, Bodies: []string{"null", `{"reason":"incident"}{"reason":"ignored"}`}, Setup: base("kill-activate")},
		{Name: "kill-activate-required-body-errors", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-kill-switch/activate", "/api/v1/system/real-trade-kill-switch/activate", "/api/v1/system/real-trade-kill-switch/activate"}, Bodies: []string{"", "{", "[]"}, Setup: base("kill-activate")},
		{Name: "kill-activate-control-failure", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-kill-switch/activate"}, Bodies: []string{"{}"}, Setup: func(t *testing.T) *stage9SystemWriteHarness {
			h := stage9SystemWriteBase(t, "kill-activate")
			h.controls.killOnErr = errors.New("kill switch persistence unavailable")
			return h
		}},
		{Name: "kill-activate-canceled", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-kill-switch/activate"}, Bodies: []string{"{}"}, ContextError: "canceled", Setup: withContext("kill-activate")},
		{Name: "kill-activate-deadline", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-kill-switch/activate"}, Bodies: []string{"{}"}, ContextError: "deadline", Setup: withContext("kill-activate")},
		{Name: "kill-activate-repeated", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-kill-switch/activate", "/api/v1/system/real-trade-kill-switch/activate"}, Bodies: []string{"{}", "{}"}, Setup: base("kill-activate")},

		{Name: "kill-release-empty-and-null", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-kill-switch/release", "/api/v1/system/real-trade-kill-switch/release"}, Bodies: []string{"", "null"}, Setup: base("kill-release")},
		{Name: "kill-release-trailing", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-kill-switch/release"}, Bodies: []string{`{"operatorId":"operator"}{"reason":"ignored"}`}, Setup: base("kill-release")},
		{Name: "kill-release-malformed-before-port", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-kill-switch/release"}, Bodies: []string{"{"}, Setup: base("kill-release")},
		{Name: "kill-release-control-failure", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-kill-switch/release"}, Bodies: []string{"{}"}, Setup: func(t *testing.T) *stage9SystemWriteHarness {
			h := stage9SystemWriteBase(t, "kill-release")
			h.controls.killOffErr = errors.New("kill switch is not active")
			return h
		}},
		{Name: "kill-release-canceled", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-kill-switch/release"}, Bodies: []string{"{}"}, ContextError: "canceled", Setup: withContext("kill-release")},
		{Name: "kill-release-deadline", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-kill-switch/release"}, Bodies: []string{"{}"}, ContextError: "deadline", Setup: withContext("kill-release")},
		{Name: "kill-release-repeated", Method: http.MethodPost, Paths: []string{"/api/v1/system/real-trade-kill-switch/release", "/api/v1/system/real-trade-kill-switch/release"}, Bodies: []string{"", ""}, Setup: base("kill-release")},

		{Name: "risk-update-success-unknown-field", Method: http.MethodPut, Paths: []string{"/api/v1/system/real-trade-risk-limits"}, Bodies: []string{`{"tradingEnvironment":"REAL","realTradingEnabled":true,"maxOrderQuantity":12.5,"operatorId":"operator","reason":"session open","unknownField":true}`}, Setup: base("risk-update")},
		{Name: "risk-update-null-and-trailing", Method: http.MethodPut, Paths: []string{"/api/v1/system/real-trade-risk-limits", "/api/v1/system/real-trade-risk-limits"}, Bodies: []string{"null", `{"realTradingEnabled":true,"maxOrderNotional":2500}{"reason":"ignored"}`}, Setup: base("risk-update")},
		{Name: "risk-update-required-body-errors", Method: http.MethodPut, Paths: []string{"/api/v1/system/real-trade-risk-limits", "/api/v1/system/real-trade-risk-limits", "/api/v1/system/real-trade-risk-limits"}, Bodies: []string{"", "{", "[]"}, Setup: base("risk-update")},
		{Name: "risk-update-validation-errors", Method: http.MethodPut, Paths: []string{"/api/v1/system/real-trade-risk-limits", "/api/v1/system/real-trade-risk-limits", "/api/v1/system/real-trade-risk-limits"}, Bodies: []string{`{"maxOrderQuantity":0}`, `{"maxOrderNotional":-1}`, `{"realTradingEnabled":true}`}, Setup: base("risk-update")},
		{Name: "risk-update-control-failure", Method: http.MethodPut, Paths: []string{"/api/v1/system/real-trade-risk-limits"}, Bodies: []string{`{"maxOrderQuantity":1}`}, Setup: func(t *testing.T) *stage9SystemWriteHarness {
			h := stage9SystemWriteBase(t, "risk-update")
			h.controls.updateErr = errors.New("risk configuration persistence unavailable")
			return h
		}},
		{Name: "risk-update-canceled", Method: http.MethodPut, Paths: []string{"/api/v1/system/real-trade-risk-limits"}, Bodies: []string{`{"maxOrderQuantity":1}`}, ContextError: "canceled", Setup: withContext("risk-update")},
		{Name: "risk-update-deadline", Method: http.MethodPut, Paths: []string{"/api/v1/system/real-trade-risk-limits"}, Bodies: []string{`{"maxOrderQuantity":1}`}, ContextError: "deadline", Setup: withContext("risk-update")},
		{Name: "risk-update-repeated", Method: http.MethodPut, Paths: []string{"/api/v1/system/real-trade-risk-limits", "/api/v1/system/real-trade-risk-limits"}, Bodies: []string{`{"maxOrderQuantity":1}`, `{"maxOrderQuantity":1}`}, Setup: base("risk-update")},

		{Name: "risk-disable-empty-and-null", Method: http.MethodDelete, Paths: []string{"/api/v1/system/real-trade-risk-limits", "/api/v1/system/real-trade-risk-limits"}, Bodies: []string{"", "null"}, Setup: base("risk-disable")},
		{Name: "risk-disable-trailing", Method: http.MethodDelete, Paths: []string{"/api/v1/system/real-trade-risk-limits"}, Bodies: []string{`{"operatorId":"operator"}{"reason":"ignored"}`}, Setup: base("risk-disable")},
		{Name: "risk-disable-malformed-before-port", Method: http.MethodDelete, Paths: []string{"/api/v1/system/real-trade-risk-limits"}, Bodies: []string{"{"}, Setup: base("risk-disable")},
		{Name: "risk-disable-control-failure", Method: http.MethodDelete, Paths: []string{"/api/v1/system/real-trade-risk-limits"}, Bodies: []string{"{}"}, Setup: func(t *testing.T) *stage9SystemWriteHarness {
			h := stage9SystemWriteBase(t, "risk-disable")
			h.controls.disableErr = errors.New("risk configuration is not active")
			return h
		}},
		{Name: "risk-disable-canceled", Method: http.MethodDelete, Paths: []string{"/api/v1/system/real-trade-risk-limits"}, Bodies: []string{"{}"}, ContextError: "canceled", Setup: withContext("risk-disable")},
		{Name: "risk-disable-deadline", Method: http.MethodDelete, Paths: []string{"/api/v1/system/real-trade-risk-limits"}, Bodies: []string{"{}"}, ContextError: "deadline", Setup: withContext("risk-disable")},
		{Name: "risk-disable-repeated", Method: http.MethodDelete, Paths: []string{"/api/v1/system/real-trade-risk-limits", "/api/v1/system/real-trade-risk-limits"}, Bodies: []string{"", ""}, Setup: base("risk-disable")},
	}
}

func TestStage9SystemWriteFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve system-write fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/system-write.json",
	)
	want := stage9SystemWriteFixture{
		Version: stage9SystemWriteFixtureVersion,
		Cases:   make([]stage9SystemWriteFixtureCase, 0),
	}
	for _, spec := range stage9SystemWriteCaseSpecs() {
		want.Cases = append(want.Cases, stage9RunSystemWriteCase(t, spec))
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode system-write fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write system-write fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read system-write fixture: %v", err)
	}
	var got stage9SystemWriteFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode system-write fixture: %v", err)
	}
	compactStage9SystemWriteFixture(&got)
	compactStage9SystemWriteFixture(&want)
	wantBytes, _ := json.Marshal(want)
	gotBytes, _ := json.Marshal(got)
	if !bytes.Equal(gotBytes, wantBytes) {
		t.Fatalf("system-write fixture drifted: want=%s got=%s", wantBytes, gotBytes)
	}
}

func compactStage9SystemWriteFixture(fixture *stage9SystemWriteFixture) {
	for caseIndex := range fixture.Cases {
		for responseIndex, response := range fixture.Cases[caseIndex].Responses {
			var compacted bytes.Buffer
			if err := json.Compact(&compacted, response); err == nil {
				fixture.Cases[caseIndex].Responses[responseIndex] = append(
					json.RawMessage(nil), compacted.Bytes()...,
				)
			}
		}
	}
}
