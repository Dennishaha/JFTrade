package rustmigration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/futuapp"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	appruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	desktopapp "github.com/jftrade/jftrade-main/internal/desktop"
	"github.com/jftrade/jftrade-main/internal/integration/akshare"
	"github.com/jftrade/jftrade-main/internal/integration/yfinance"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/live"
	srvsettings "github.com/jftrade/jftrade-main/internal/settings"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	sysservice "github.com/jftrade/jftrade-main/internal/system"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

type stage9ProductCorpus struct {
	Version         string `json:"version"`
	AppearanceCases []struct {
		Name     string                          `json:"name"`
		Input    jfsettings.UIAppearanceSettings `json:"input"`
		Expected jfsettings.UIAppearanceSettings `json:"expected"`
	} `json:"appearanceCases"`
	OnboardingCases []struct {
		Name                  string          `json:"name"`
		Input                 json.RawMessage `json:"input"`
		DependenciesSatisfied bool            `json:"dependenciesSatisfied"`
		Expected              json.RawMessage `json:"expected"`
	} `json:"onboardingCases"`
	FutuInstallCases []struct {
		Name     string          `json:"name"`
		Input    json.RawMessage `json:"input"`
		Expected json.RawMessage `json:"expected"`
	} `json:"futuInstallCases"`
	ExecutionCases []struct {
		Name     string                       `json:"name"`
		Input    jfsettings.ExecutionSettings `json:"input"`
		Expected jfsettings.ExecutionSettings `json:"expected"`
	} `json:"executionCases"`
	SecurityCases []struct {
		Name  string `json:"name"`
		Input struct {
			WebAccessEnabled    bool   `json:"webAccessEnabled"`
			PublicAccessEnabled bool   `json:"publicAccessEnabled"`
			WebPort             int    `json:"webPort"`
			PasswordHash        string `json:"passwordHash"`
		} `json:"input"`
		Expected jfsettings.SecuritySettings `json:"expected"`
	} `json:"securityCases"`
	MarketDataProviderCases []struct {
		Name     string                              `json:"name"`
		Input    jfsettings.ActiveMarketDataProvider `json:"input"`
		Expected jfsettings.ActiveMarketDataProvider `json:"expected"`
	} `json:"marketDataProviderCases"`
	BacktestMarketDataProviderCases []struct {
		Name             string                               `json:"name"`
		ActiveProvider   *jfsettings.ActiveMarketDataProvider `json:"activeProvider"`
		BacktestProvider *jfsettings.ActiveMarketDataProvider `json:"backtestProvider"`
		Expected         jfsettings.ActiveMarketDataProvider  `json:"expected"`
	} `json:"backtestMarketDataProviderCases"`
	ExchangeCalendarCases []struct {
		Name     string          `json:"name"`
		Input    json.RawMessage `json:"input"`
		Expected json.RawMessage `json:"expected"`
	} `json:"exchangeCalendarCases"`
	AssistantRuntimeCases []struct {
		Name     string                        `json:"name"`
		Input    jfsettings.ADKRuntimeSettings `json:"input"`
		Expected jfsettings.ADKRuntimeSettings `json:"expected"`
	} `json:"assistantRuntimeCases"`
	MCPServerCases []struct {
		Name  string `json:"name"`
		Input struct {
			Enabled   bool   `json:"enabled"`
			Port      int    `json:"port"`
			AuthMode  string `json:"authMode"`
			TokenHash string `json:"tokenHash"`
		} `json:"input"`
		Expected jfsettings.MCPServerSettings `json:"expected"`
	} `json:"mcpServerCases"`
	SystemNotificationCases []struct {
		Name     string                                `json:"name"`
		Input    jfsettings.SystemNotificationSettings `json:"input"`
		Expected json.RawMessage                       `json:"expected"`
	} `json:"systemNotificationCases"`
	PineWorkerCases []struct {
		Name     string                        `json:"name"`
		Input    jfsettings.PineWorkerSettings `json:"input"`
		Expected jfsettings.PineWorkerSettings `json:"expected"`
	} `json:"pineWorkerCases"`
	NotificationForwardCases []struct {
		Name     string                                `json:"name"`
		Settings jfsettings.SystemNotificationSettings `json:"settings"`
		Level    string                                `json:"level"`
		Category string                                `json:"category"`
		Expected bool                                  `json:"expected"`
	} `json:"notificationForwardCases"`
	NodeVersionCases []struct {
		Name                    string `json:"name"`
		Output                  string `json:"output"`
		ExpectedStatus          string `json:"expectedStatus"`
		ExpectedDetectedVersion string `json:"expectedDetectedVersion"`
		ExpectedMessage         string `json:"expectedMessage"`
	} `json:"nodeVersionCases"`
}

func TestStage9AppearanceCorpusMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 Go reference source")
	}
	path := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/product-slice-corpus.json",
	)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stage 9 corpus: %v", err)
	}
	var corpus stage9ProductCorpus
	if err := json.Unmarshal(contents, &corpus); err != nil {
		t.Fatalf("decode stage 9 corpus: %v", err)
	}
	if corpus.Version != "stage9.product-slice.v10" || len(corpus.AppearanceCases) < 4 || len(corpus.OnboardingCases) < 4 || len(corpus.FutuInstallCases) < 4 || len(corpus.ExecutionCases) < 4 || len(corpus.SecurityCases) < 4 || len(corpus.MarketDataProviderCases) < 5 || len(corpus.BacktestMarketDataProviderCases) < 4 || len(corpus.ExchangeCalendarCases) < 4 || len(corpus.AssistantRuntimeCases) < 4 || len(corpus.MCPServerCases) < 4 || len(corpus.SystemNotificationCases) < 4 || len(corpus.PineWorkerCases) < 4 || len(corpus.NotificationForwardCases) < 5 || len(corpus.NodeVersionCases) < 4 {
		t.Fatalf("stage 9 corpus is incomplete: version=%q cases=%d", corpus.Version, len(corpus.AppearanceCases))
	}
	for _, testCase := range corpus.AppearanceCases {
		t.Run(testCase.Name, func(t *testing.T) {
			if got := settingsfile.NormalizeUIAppearanceSettings(testCase.Input); got != testCase.Expected {
				t.Fatalf("normalized appearance = %#v, want %#v", got, testCase.Expected)
			}
		})
	}
	for _, testCase := range corpus.OnboardingCases {
		t.Run("onboarding-"+testCase.Name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(path, testCase.Input, 0o600); err != nil {
				t.Fatalf("seed onboarding document: %v", err)
			}
			store, err := settingsfile.New(path)
			if err != nil {
				t.Fatalf("open onboarding document: %v", err)
			}
			coordinator := futuapp.New(futuapp.Options{
				Settings: store,
				RuntimeDependencies: func(context.Context) map[string]any {
					return map[string]any{"allRequiredSatisfied": testCase.DependenciesSatisfied}
				},
			})
			state := coordinator.OnboardingState(t.Context())
			broker := state["brokers"].([]any)[0].(map[string]any)
			actual := map[string]any{
				"state": state["state"], "shouldShowOobe": state["shouldShowOobe"],
				"reasons": state["reasons"], "brokerEnabled": broker["enabled"],
				"brokerConfigured": broker["configured"],
			}
			assertStage9JSONEqual(t, actual, testCase.Expected, "onboarding")
		})
	}
	for _, testCase := range corpus.FutuInstallCases {
		t.Run("futu-install-"+testCase.Name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(path, testCase.Input, 0o600); err != nil {
				t.Fatalf("seed Futu install document: %v", err)
			}
			store, err := settingsfile.New(path)
			if err != nil {
				t.Fatalf("open Futu install document: %v", err)
			}
			guide := futuapp.New(futuapp.Options{Settings: store}).OpenDInstallGuide()
			assertStage9JSONEqual(t, guide["settings"], testCase.Expected, "Futu install settings")
		})
	}
	for _, testCase := range corpus.ExecutionCases {
		t.Run("execution-"+testCase.Name, func(t *testing.T) {
			if got := settingsfile.NormalizeExecutionSettings(testCase.Input); got != testCase.Expected {
				t.Fatalf("normalized execution = %#v, want %#v", got, testCase.Expected)
			}
		})
	}
	for _, testCase := range corpus.SecurityCases {
		t.Run("security-"+testCase.Name, func(t *testing.T) {
			got := settingsfile.NormalizeSecuritySettings(jfsettings.SecuritySettings{
				WebAccessEnabled:    testCase.Input.WebAccessEnabled,
				PublicAccessEnabled: testCase.Input.PublicAccessEnabled,
				WebPort:             testCase.Input.WebPort,
				PasswordHash:        testCase.Input.PasswordHash,
			})
			got.PasswordHash = ""
			if got != testCase.Expected {
				t.Fatalf("normalized security projection = %#v, want %#v", got, testCase.Expected)
			}
		})
	}
	for _, testCase := range corpus.MarketDataProviderCases {
		t.Run("market-data-provider-"+testCase.Name, func(t *testing.T) {
			if got := settingsfile.NormalizeActiveMarketDataProvider(testCase.Input); got != testCase.Expected {
				t.Fatalf("normalized market-data provider = %q, want %q", got, testCase.Expected)
			}
		})
	}
	for _, testCase := range corpus.BacktestMarketDataProviderCases {
		t.Run("backtest-market-data-provider-"+testCase.Name, func(t *testing.T) {
			document := map[string]any{}
			if testCase.ActiveProvider != nil {
				document["activeMarketDataProvider"] = *testCase.ActiveProvider
			}
			if testCase.BacktestProvider != nil {
				document["backtestMarketDataProvider"] = *testCase.BacktestProvider
			}
			path := filepath.Join(t.TempDir(), "settings.json")
			contents, err := json.Marshal(document)
			if err != nil {
				t.Fatalf("encode backtest provider document: %v", err)
			}
			if err := os.WriteFile(path, contents, 0o600); err != nil {
				t.Fatalf("seed backtest provider document: %v", err)
			}
			store, err := settingsfile.New(path)
			if err != nil {
				t.Fatalf("open backtest provider document: %v", err)
			}
			if got := store.BacktestMarketDataProvider(); got != testCase.Expected {
				t.Fatalf("backtest provider = %q, want %q", got, testCase.Expected)
			}
		})
	}
	for _, testCase := range corpus.ExchangeCalendarCases {
		t.Run("exchange-calendar-"+testCase.Name, func(t *testing.T) {
			var input jfsettings.ExchangeCalendarSettings
			if err := json.Unmarshal(testCase.Input, &input); err != nil {
				t.Fatalf("decode calendar input: %v", err)
			}
			encoded, err := json.Marshal(settingsfile.NormalizeExchangeCalendarSettings(input))
			if err != nil {
				t.Fatalf("encode normalized calendar settings: %v", err)
			}
			var gotJSON any
			var wantJSON any
			if err := json.Unmarshal(encoded, &gotJSON); err != nil {
				t.Fatalf("decode normalized calendar settings: %v", err)
			}
			if err := json.Unmarshal(testCase.Expected, &wantJSON); err != nil {
				t.Fatalf("decode expected calendar settings: %v", err)
			}
			if !reflect.DeepEqual(gotJSON, wantJSON) {
				t.Fatalf("normalized calendar settings = %#v, want %#v", gotJSON, wantJSON)
			}
		})
	}
	for _, testCase := range corpus.AssistantRuntimeCases {
		t.Run("assistant-runtime-"+testCase.Name, func(t *testing.T) {
			if got := settingsfile.NormalizeADKRuntimeSettings(testCase.Input); got != testCase.Expected {
				t.Fatalf("normalized assistant runtime = %#v, want %#v", got, testCase.Expected)
			}
		})
	}
	for _, testCase := range corpus.MCPServerCases {
		t.Run("mcp-server-"+testCase.Name, func(t *testing.T) {
			got := settingsfile.NormalizeMCPServerSettings(jfsettings.MCPServerSettings{
				Enabled: testCase.Input.Enabled, Port: testCase.Input.Port,
				AuthMode: testCase.Input.AuthMode, TokenHash: testCase.Input.TokenHash,
			})
			got.TokenHash = ""
			if got != testCase.Expected {
				t.Fatalf("normalized MCP server projection = %#v, want %#v", got, testCase.Expected)
			}
		})
	}
	for _, testCase := range corpus.SystemNotificationCases {
		t.Run("system-notifications-"+testCase.Name, func(t *testing.T) {
			got := settingsfile.NormalizeSystemNotificationSettings(testCase.Input)
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("encode normalized system notifications: %v", err)
			}
			var gotJSON any
			var wantJSON any
			if err := json.Unmarshal(encoded, &gotJSON); err != nil {
				t.Fatalf("decode normalized system notifications: %v", err)
			}
			if err := json.Unmarshal(testCase.Expected, &wantJSON); err != nil {
				t.Fatalf("decode expected system notifications: %v", err)
			}
			if !reflect.DeepEqual(gotJSON, wantJSON) {
				t.Fatalf("normalized system notifications = %#v, want %#v", gotJSON, wantJSON)
			}
		})
	}
	for _, testCase := range corpus.PineWorkerCases {
		t.Run("pine-worker-"+testCase.Name, func(t *testing.T) {
			if got := settingsfile.NormalizePineWorkerSettings(testCase.Input); got != testCase.Expected {
				t.Fatalf("normalized Pine worker settings = %#v, want %#v", got, testCase.Expected)
			}
		})
	}
	for _, testCase := range corpus.NodeVersionCases {
		t.Run("node-"+testCase.Name, func(t *testing.T) {
			restore := appruntime.OverrideDependencyProbe(
				func(path string) (string, error) { return path, nil },
				func(context.Context, string, ...string) ([]byte, error) {
					return []byte(testCase.Output), nil
				},
			)
			defer restore()
			result := appruntime.CheckNodeRuntimeDependency(
				context.Background(),
				jfsettings.PineWorkerSettings{NodeBinaryPath: "/fixture/node"},
			)
			if result["status"] != testCase.ExpectedStatus ||
				result["detectedVersion"] != testCase.ExpectedDetectedVersion ||
				result["message"] != testCase.ExpectedMessage {
				t.Fatalf("node projection = %#v", result)
			}
		})
	}
	for _, testCase := range corpus.NotificationForwardCases {
		t.Run("notification-forward-"+testCase.Name, func(t *testing.T) {
			got := desktopapp.ShouldForwardSystemNotification(testCase.Settings, live.Event{
				Level: testCase.Level, Category: testCase.Category,
			})
			if got != testCase.Expected {
				t.Fatalf("notification forwarding = %v, want %v", got, testCase.Expected)
			}
		})
	}
}

func TestStage9ProviderDescriptorsMatchCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 Go reference source")
	}
	path := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/provider-descriptors.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read provider descriptor fixture: %v", err)
	}
	var expected any
	if err := json.Unmarshal(contents, &expected); err != nil {
		t.Fatalf("decode provider descriptor fixture: %v", err)
	}
	futu, err := marketdataapp.FutuProviderDescriptor(t.Context())
	if err != nil {
		t.Fatalf("Futu provider descriptor: %v", err)
	}
	encoded, err := json.Marshal([]any{futu, yfinance.ProviderDescriptor(), akshare.ProviderDescriptor()})
	if err != nil {
		t.Fatalf("encode Go provider descriptors: %v", err)
	}
	var actual any
	if err := json.Unmarshal(encoded, &actual); err != nil {
		t.Fatalf("decode Go provider descriptors: %v", err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Go provider descriptors diverged from Stage 9 fixture")
	}
}

func TestStage9BrokerDescriptorMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 Go reference source")
	}
	path := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/broker-descriptor.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read broker descriptor fixture: %v", err)
	}
	assertStage9JSONEqual(t, futuapp.BrokerRuntimeDescriptor(), contents, "broker descriptor")
}

type stage9BrokerSettingsCorpus struct {
	Version string `json:"version"`
	Cases   []struct {
		Name     string          `json:"name"`
		Document json.RawMessage `json:"document"`
	} `json:"cases"`
}

func TestStage9BrokerSettingsReadReference(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 Go reference source")
	}
	path := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/broker-settings-corpus.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read broker settings corpus: %v", err)
	}
	var corpus stage9BrokerSettingsCorpus
	if err := json.Unmarshal(contents, &corpus); err != nil {
		t.Fatalf("decode broker settings corpus: %v", err)
	}
	if corpus.Version != "stage9.broker-settings-read.v1" || len(corpus.Cases) < 4 {
		t.Fatalf("broker settings corpus is incomplete: version=%q cases=%d", corpus.Version, len(corpus.Cases))
	}
	results := make([]map[string]any, 0, len(corpus.Cases))
	for _, testCase := range corpus.Cases {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, testCase.Document, 0o600); err != nil {
			t.Fatalf("seed broker settings document: %v", err)
		}
		store, err := settingsfile.New(path)
		if err != nil {
			t.Fatalf("open broker settings document: %v", err)
		}
		projection := futuapp.New(futuapp.Options{Settings: store}).BrokerSettings()
		results = append(results, map[string]any{"name": testCase.Name, "projection": projection})
	}
	outputPath := os.Getenv("JFTRADE_STAGE9_BROKER_SETTINGS_REFERENCE")
	if outputPath == "" {
		return
	}
	encoded, err := json.Marshal(map[string]any{"version": corpus.Version, "results": results})
	if err != nil {
		t.Fatalf("encode broker settings reference: %v", err)
	}
	if err := os.WriteFile(outputPath, encoded, 0o600); err != nil {
		t.Fatalf("write broker settings reference: %v", err)
	}
}

type stage9BrokerSettingsWriteCorpus struct {
	Version      string                          `json:"version"`
	SeedDocument json.RawMessage                 `json:"seedDocument"`
	Integration  jfsettings.BrokerIntegration    `json:"integration"`
	CreateFirst  jfsettings.ManagedBrokerAccount `json:"createFirst"`
	UpsertFirst  jfsettings.ManagedBrokerAccount `json:"upsertFirst"`
	CreateSecond jfsettings.ManagedBrokerAccount `json:"createSecond"`
	UpdateSecond jfsettings.ManagedBrokerAccount `json:"updateSecond"`
	DeleteID     string                          `json:"deleteId"`
	MissingID    string                          `json:"missingId"`
}

func TestStage9BrokerSettingsWriteReference(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 Go reference source")
	}
	path := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/broker-settings-write-corpus.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read broker settings write corpus: %v", err)
	}
	var corpus stage9BrokerSettingsWriteCorpus
	if err := json.Unmarshal(contents, &corpus); err != nil {
		t.Fatalf("decode broker settings write corpus: %v", err)
	}
	if corpus.Version != "stage9.broker-settings-write.v1" {
		t.Fatalf("unexpected broker settings write corpus %q", corpus.Version)
	}
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settingsPath, corpus.SeedDocument, 0o600); err != nil {
		t.Fatalf("seed broker settings write document: %v", err)
	}
	store, err := settingsfile.New(settingsPath)
	if err != nil {
		t.Fatalf("open broker settings write document: %v", err)
	}
	svc := srvsettings.NewService(store)
	integration, err := svc.SaveIntegration(corpus.Integration)
	if err != nil {
		t.Fatalf("save integration: %v", err)
	}
	createdFirst, err := svc.CreateManagedAccount(corpus.CreateFirst)
	if err != nil {
		t.Fatalf("create first account: %v", err)
	}
	upsertedFirst, err := svc.CreateManagedAccount(corpus.UpsertFirst)
	if err != nil {
		t.Fatalf("upsert first account: %v", err)
	}
	createdSecond, err := svc.CreateManagedAccount(corpus.CreateSecond)
	if err != nil {
		t.Fatalf("create second account: %v", err)
	}
	updatedSecond, err := svc.UpdateManagedAccount(createdSecond.ID, corpus.UpdateSecond)
	if err != nil {
		t.Fatalf("update second account: %v", err)
	}
	if err := svc.DeleteManagedAccount(corpus.DeleteID); err != nil {
		t.Fatalf("delete first account: %v", err)
	}
	_, updateMissingErr := svc.UpdateManagedAccount(corpus.MissingID, corpus.UpdateSecond)
	deleteMissingErr := svc.DeleteManagedAccount(corpus.MissingID)
	projection := futuapp.New(futuapp.Options{Settings: store}).BrokerSettings()
	persisted, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read persisted broker settings: %v", err)
	}
	var persistedDocument any
	if err := json.Unmarshal(persisted, &persistedDocument); err != nil {
		t.Fatalf("decode persisted broker settings: %v", err)
	}
	result := map[string]any{
		"version":       corpus.Version,
		"integration":   integration,
		"createdFirst":  createdFirst,
		"upsertedFirst": upsertedFirst,
		"createdSecond": createdSecond,
		"updatedSecond": updatedSecond,
		"updateMissing": updateMissingErr != nil,
		"deleteMissing": deleteMissingErr != nil,
		"projection":    projection,
		"persisted":     persistedDocument,
	}
	outputPath := os.Getenv("JFTRADE_STAGE9_BROKER_SETTINGS_WRITE_REFERENCE")
	if outputPath == "" {
		return
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encode broker settings write reference: %v", err)
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		t.Fatalf("decode broker settings write reference: %v", err)
	}
	normalizeStage9BrokerTimestamps(normalized)
	encoded, err = json.Marshal(normalized)
	if err != nil {
		t.Fatalf("encode normalized broker settings write reference: %v", err)
	}
	if err := os.WriteFile(outputPath, encoded, 0o600); err != nil {
		t.Fatalf("write broker settings write reference: %v", err)
	}
}

func normalizeStage9BrokerTimestamps(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if (key == "createdAt" || key == "updatedAt" || key == "completedAt" || key == "dismissedAt") && item != "" {
				typed[key] = "<timestamp>"
				continue
			}
			normalizeStage9BrokerTimestamps(item)
		}
	case []any:
		for _, item := range typed {
			normalizeStage9BrokerTimestamps(item)
		}
	}
}

type stage9OnboardingSettingsWriteCorpus struct {
	Version string `json:"version"`
	Cases   []struct {
		Name         string          `json:"name"`
		SeedDocument json.RawMessage `json:"seedDocument"`
		Input        struct {
			Completed    bool   `json:"completed"`
			Dismissed    bool   `json:"dismissed"`
			LastBrokerID string `json:"lastBrokerId"`
		} `json:"input"`
	} `json:"cases"`
}

func TestStage9OnboardingSettingsWriteReference(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 Go reference source")
	}
	path := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/onboarding-settings-write-corpus.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read onboarding settings write corpus: %v", err)
	}
	var corpus stage9OnboardingSettingsWriteCorpus
	if err := json.Unmarshal(contents, &corpus); err != nil {
		t.Fatalf("decode onboarding settings write corpus: %v", err)
	}
	if corpus.Version != "stage9.onboarding-settings-write.v1" || len(corpus.Cases) < 4 {
		t.Fatalf("onboarding settings write corpus is incomplete: version=%q cases=%d", corpus.Version, len(corpus.Cases))
	}
	results := make([]map[string]any, 0, len(corpus.Cases))
	for _, testCase := range corpus.Cases {
		settingsPath := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(settingsPath, testCase.SeedDocument, 0o600); err != nil {
			t.Fatalf("seed onboarding settings write document: %v", err)
		}
		store, err := settingsfile.New(settingsPath)
		if err != nil {
			t.Fatalf("open onboarding settings write document: %v", err)
		}
		existing := store.Onboarding()
		now := time.Now().UTC().Format(time.RFC3339Nano)
		next := existing
		next.LastBrokerID = testCase.Input.LastBrokerID
		if strings.TrimSpace(next.LastBrokerID) == "" {
			next.LastBrokerID = existing.LastBrokerID
		}
		if testCase.Input.Completed || testCase.Input.Dismissed {
			next.Completed = true
			if testCase.Input.Dismissed {
				next.DismissedAt = now
			}
			if next.CompletedAt == "" {
				next.CompletedAt = now
			}
		} else {
			next.Completed = false
			next.CompletedAt = ""
			next.DismissedAt = ""
		}
		saved, err := store.SaveOnboarding(next)
		if err != nil {
			t.Fatalf("save onboarding settings: %v", err)
		}
		persisted, err := os.ReadFile(settingsPath)
		if err != nil {
			t.Fatalf("read persisted onboarding settings: %v", err)
		}
		var persistedDocument any
		if err := json.Unmarshal(persisted, &persistedDocument); err != nil {
			t.Fatalf("decode persisted onboarding settings: %v", err)
		}
		results = append(results, map[string]any{
			"name":      testCase.Name,
			"saved":     saved,
			"persisted": persistedDocument,
		})
	}
	result := map[string]any{"version": corpus.Version, "results": results}
	outputPath := os.Getenv("JFTRADE_STAGE9_ONBOARDING_SETTINGS_WRITE_REFERENCE")
	if outputPath == "" {
		return
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encode onboarding settings write reference: %v", err)
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		t.Fatalf("decode onboarding settings write reference: %v", err)
	}
	normalizeStage9BrokerTimestamps(normalized)
	encoded, err = json.Marshal(normalized)
	if err != nil {
		t.Fatalf("encode normalized onboarding settings write reference: %v", err)
	}
	if err := os.WriteFile(outputPath, encoded, 0o600); err != nil {
		t.Fatalf("write onboarding settings write reference: %v", err)
	}
}

type stage9ProviderSettingsWriteCorpus struct {
	Version        string                                `json:"version"`
	SeedDocument   json.RawMessage                       `json:"seedDocument"`
	ActiveInputs   []jfsettings.ActiveMarketDataProvider `json:"activeInputs"`
	BacktestInputs []jfsettings.ActiveMarketDataProvider `json:"backtestInputs"`
}

func TestStage9ProviderSettingsWriteReference(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 Go reference source")
	}
	path := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/provider-settings-write-corpus.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read provider settings write corpus: %v", err)
	}
	var corpus stage9ProviderSettingsWriteCorpus
	if err := json.Unmarshal(contents, &corpus); err != nil {
		t.Fatalf("decode provider settings write corpus: %v", err)
	}
	if corpus.Version != "stage9.provider-settings-write.v1" || len(corpus.ActiveInputs) < 3 || len(corpus.BacktestInputs) < 3 {
		t.Fatalf("provider settings write corpus is incomplete: version=%q", corpus.Version)
	}
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settingsPath, corpus.SeedDocument, 0o600); err != nil {
		t.Fatalf("seed provider settings write document: %v", err)
	}
	store, err := settingsfile.New(settingsPath)
	if err != nil {
		t.Fatalf("open provider settings write document: %v", err)
	}
	service := srvsettings.NewService(store)
	activeResults := make([]map[string]any, 0, len(corpus.ActiveInputs))
	for _, input := range corpus.ActiveInputs {
		provider, saveErr := service.SaveActiveMarketDataProvider(input)
		activeResults = append(activeResults, map[string]any{
			"input": input, "provider": provider, "error": saveErr != nil,
		})
	}
	backtestResults := make([]map[string]any, 0, len(corpus.BacktestInputs))
	for _, input := range corpus.BacktestInputs {
		provider, saveErr := service.SaveBacktestMarketDataProvider(input)
		backtestResults = append(backtestResults, map[string]any{
			"input": input, "provider": provider.ActiveProvider, "error": saveErr != nil,
		})
	}
	persisted, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read persisted provider settings: %v", err)
	}
	var persistedDocument any
	if err := json.Unmarshal(persisted, &persistedDocument); err != nil {
		t.Fatalf("decode persisted provider settings: %v", err)
	}
	result := map[string]any{
		"version": corpus.Version, "activeResults": activeResults,
		"backtestResults": backtestResults, "persisted": persistedDocument,
	}
	outputPath := os.Getenv("JFTRADE_STAGE9_PROVIDER_SETTINGS_WRITE_REFERENCE")
	if outputPath == "" {
		return
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encode provider settings write reference: %v", err)
	}
	if err := os.WriteFile(outputPath, encoded, 0o600); err != nil {
		t.Fatalf("write provider settings write reference: %v", err)
	}
}

type stage9RealTradeCorpus struct {
	Version string `json:"version"`
	Cases   []struct {
		Name                string  `json:"name"`
		Document            *string `json:"document"`
		ExpectedUnavailable bool    `json:"expectedUnavailable"`
	} `json:"cases"`
}

func TestStage9RealTradeReadReference(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 Go reference source")
	}
	path := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/real-trade-control-corpus.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read real-trade corpus: %v", err)
	}
	var corpus stage9RealTradeCorpus
	if err := json.Unmarshal(contents, &corpus); err != nil {
		t.Fatalf("decode real-trade corpus: %v", err)
	}
	if corpus.Version != "stage9.real-trade-read.v1" || len(corpus.Cases) < 5 {
		t.Fatalf("real-trade corpus is incomplete: version=%q cases=%d", corpus.Version, len(corpus.Cases))
	}

	results := make([]map[string]any, 0, len(corpus.Cases))
	for _, testCase := range corpus.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			controlPath := filepath.Join(t.TempDir(), "real-trade-control.json")
			if testCase.Document != nil {
				if err := os.WriteFile(controlPath, []byte(*testCase.Document), 0o600); err != nil {
					t.Fatalf("seed real-trade control document: %v", err)
				}
			}
			plane, loadErr := trdsrv.NewRealTradeControlPlane(controlPath)
			if (loadErr != nil) != testCase.ExpectedUnavailable {
				t.Fatalf("control unavailable = %v, want %v: %v", loadErr != nil, testCase.ExpectedUnavailable, loadErr)
			}
			snapshot := plane.Snapshot()
			svc := sysservice.NewService(sysservice.WithRealTradeRiskState(func() *trdsrv.RealTradeRiskSnapshot {
				return &snapshot
			}))
			status := svc.Status()
			results = append(results, map[string]any{
				"name":                  testCase.Name,
				"controlPlaneAvailable": snapshot.ControlPlaneAvailable,
				"status": map[string]any{
					"realTradingEnabled":    status.RealTradingEnabled,
					"realTradingKillSwitch": status.RealTradingKillSwitch,
					"realTradingRisk":       status.RealTradingRisk,
				},
				"approvals":        svc.RealTradeApprovals(),
				"hardStops":        svc.RealTradeHardStops(),
				"hardStopEvents":   svc.RealTradeHardStopEvents(),
				"killSwitch":       svc.RealTradeKillSwitch(),
				"killSwitchEvents": svc.RealTradeKillSwitchEvents(),
				"riskLimits":       svc.RealTradeRiskLimits(),
				"riskEvents":       svc.RealTradeRiskEvents(),
			})
		})
	}
	outputPath := os.Getenv("JFTRADE_STAGE9_REAL_TRADE_REFERENCE")
	if outputPath == "" {
		return
	}
	encoded, err := json.Marshal(map[string]any{"version": corpus.Version, "results": results})
	if err != nil {
		t.Fatalf("encode real-trade reference: %v", err)
	}
	if err := os.WriteFile(outputPath, encoded, 0o600); err != nil {
		t.Fatalf("write real-trade reference: %v", err)
	}
}

func assertStage9JSONEqual(t *testing.T, actual any, expectedJSON []byte, label string) {
	t.Helper()
	encoded, err := json.Marshal(actual)
	if err != nil {
		t.Fatalf("encode %s: %v", label, err)
	}
	var got any
	var expected any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("decode actual %s: %v", label, err)
	}
	if err := json.Unmarshal(expectedJSON, &expected); err != nil {
		t.Fatalf("decode expected %s: %v", label, err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("%s diverged from Stage 9 fixture: got=%#v want=%#v", label, got, expected)
	}
}
