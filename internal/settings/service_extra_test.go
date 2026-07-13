package settings

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestServiceCreateManagedAccountNormalizesClientFields(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store)

	account, err := svc.CreateManagedAccount(jfsettings.ManagedBrokerAccount{
		ID:                 "client-id",
		AccountID:          "acc-1",
		CreatedAt:          "client-created",
		UpdatedAt:          "client-updated",
		TradingEnvironment: "SIMULATE",
	})
	if err != nil {
		t.Fatalf("CreateManagedAccount: %v", err)
	}
	if account.ID != "" || account.CreatedAt != "" || account.UpdatedAt != "" {
		t.Fatalf("created account = %#v", account)
	}
	if len(store.managedAccounts) != 1 || store.managedAccounts[0].AccountID != "acc-1" {
		t.Fatalf("stored accounts = %#v", store.managedAccounts)
	}
}

func TestServiceNotificationAndMCPStatusAccessors(t *testing.T) {
	store := &fakeStore{
		systemNotifications: jfsettings.SystemNotificationSettings{Enabled: true, Mode: "all"},
		mcpServer: jfsettings.MCPServerSettings{
			Enabled: true, Port: 7788, AuthMode: "none",
		},
	}
	status := jfsettings.MCPServerStatus{Running: true, Endpoint: "http://127.0.0.1:7788/mcp"}
	svc := NewService(store, WithMCPServerStatus(func() jfsettings.MCPServerStatus { return status }))

	if got := svc.GetSystemNotificationSettings(); !reflect.DeepEqual(got, store.systemNotifications) {
		t.Fatalf("GetSystemNotificationSettings = %#v", got)
	}
	updated := jfsettings.SystemNotificationSettings{Enabled: true, Mode: "custom", Levels: []string{"error"}}
	if got, err := svc.SaveSystemNotificationSettings(updated); err != nil || got.Mode != "custom" {
		t.Fatalf("SaveSystemNotificationSettings = %#v, %v", got, err)
	}
	snapshot := svc.GetMCPServerSettingsSnapshot()
	if snapshot.Settings.Port != 7788 || snapshot.Status != status {
		t.Fatalf("MCP snapshot = %#v", snapshot)
	}
}

func TestServiceDefaultMCPStatusAndTokenGeneration(t *testing.T) {
	store := &fakeStore{mcpServer: jfsettings.MCPServerSettings{Port: 7799, AuthMode: "none"}}
	snapshot := NewService(store).GetMCPServerSettingsSnapshot()
	if snapshot.Status.Endpoint != "http://127.0.0.1:7799/mcp" {
		t.Fatalf("default MCP endpoint = %q", snapshot.Status.Endpoint)
	}
	token, err := newMCPServerToken()
	if err != nil {
		t.Fatalf("newMCPServerToken: %v", err)
	}
	if !strings.HasPrefix(token, "jft_mcp_") || len(token) <= len("jft_mcp_") {
		t.Fatalf("generated MCP token = %q", token)
	}
}

func TestValidateWebAccessPasswordBoundaries(t *testing.T) {
	if !errors.Is(validateWebAccessPassword("short"), ErrWebAccessPasswordTooShort) {
		t.Fatal("short password was accepted")
	}
	if !errors.Is(validateWebAccessPassword(strings.Repeat("界", 400)), ErrWebAccessPasswordTooLong) {
		t.Fatal("oversized password was accepted")
	}
	if err := validateWebAccessPassword("123456789012345"); err != nil {
		t.Fatalf("minimum-length password rejected: %v", err)
	}
}

func TestServiceCreateManagedAccountRejectsBlankAccountID(t *testing.T) {
	svc := NewService(&fakeStore{})
	if _, err := svc.CreateManagedAccount(jfsettings.ManagedBrokerAccount{}); err == nil {
		t.Fatal("CreateManagedAccount error = nil, want bad request")
	} else if !errors.Is(err, ErrBadRequest) || err.Error() != "accountId is required" {
		t.Fatalf("CreateManagedAccount error = %v, want ErrBadRequest with field message", err)
	}
}

func TestServiceOptionsCaptureBrokerDescriptorAndDefaultTradingEnvironment(t *testing.T) {
	descriptor := map[string]any{"brokerId": "futu", "markets": []string{"HK", "US"}}
	svc := NewService(
		&fakeStore{},
		WithBrokerDescriptor(func() map[string]any { return descriptor }),
		WithDefaultTradingEnvironment("REAL"),
	)

	if svc.brokerDescriptor == nil {
		t.Fatal("broker descriptor option was not installed")
	}
	got := svc.brokerDescriptor()
	if got["brokerId"] != "futu" || len(got["markets"].([]string)) != 2 {
		t.Fatalf("broker descriptor = %#v", got)
	}
	if svc.defaultTradingEnv != "REAL" {
		t.Fatalf("default trading env = %q, want REAL", svc.defaultTradingEnv)
	}
}
