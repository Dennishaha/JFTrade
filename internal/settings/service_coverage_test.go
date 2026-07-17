package settings

import (
	"errors"
	"strings"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

type coverageErrorStore struct {
	*fakeStore
	executionErr        error
	securityErrors      []error
	mcpErrors           []error
	pineWorkerErr       error
	exchangeCalendarErr error
	integrationErr      error
}

func (s *coverageErrorStore) SaveExecutionSettings(input jfsettings.ExecutionSettings) (jfsettings.ExecutionSettings, error) {
	return input, s.executionErr
}

func (s *coverageErrorStore) SaveSecuritySettings(input jfsettings.SecuritySettings) (jfsettings.SecuritySettings, error) {
	if len(s.securityErrors) == 0 {
		return input, nil
	}
	err := s.securityErrors[0]
	s.securityErrors = s.securityErrors[1:]
	return input, err
}

func (s *coverageErrorStore) SaveMCPServerSettings(input jfsettings.MCPServerSettings) (jfsettings.MCPServerSettings, error) {
	if len(s.mcpErrors) == 0 {
		return input, nil
	}
	err := s.mcpErrors[0]
	s.mcpErrors = s.mcpErrors[1:]
	return input, err
}

func (s *coverageErrorStore) SavePineWorkerSettings(input jfsettings.PineWorkerSettings) (jfsettings.PineWorkerSettings, error) {
	return input, s.pineWorkerErr
}

func (s *coverageErrorStore) SaveExchangeCalendarSettings(input jfsettings.ExchangeCalendarSettings) (jfsettings.ExchangeCalendarSettings, error) {
	return input, s.exchangeCalendarErr
}

func (s *coverageErrorStore) SaveIntegration(input jfsettings.BrokerIntegration) (jfsettings.BrokerIntegration, error) {
	return input, s.integrationErr
}

func TestServiceCoverageForPersistenceAndMCPErrorPaths(t *testing.T) {
	persistErr := errors.New("persist failed")
	store := &coverageErrorStore{fakeStore: &fakeStore{}, executionErr: persistErr}
	svc := NewService(store)
	if _, err := svc.SaveExecutionSettings(jfsettings.ExecutionSettings{}); !errors.Is(err, persistErr) {
		t.Fatalf("SaveExecutionSettings error = %v, want %v", err, persistErr)
	}

	if _, err := svc.SaveSecuritySettings(jfsettings.SecuritySettingsUpdate{NewPassword: "short"}); !errors.Is(err, ErrWebAccessPasswordTooShort) {
		t.Fatalf("SaveSecuritySettings short password error = %v", err)
	}
	svc.hashPassword = func(string) (string, error) { return "", persistErr }
	if _, err := svc.SaveSecuritySettings(jfsettings.SecuritySettingsUpdate{NewPassword: "a password long enough"}); !errors.Is(err, persistErr) {
		t.Fatalf("SaveSecuritySettings hash error = %v", err)
	}

	store.securityErrors = []error{persistErr}
	svc.hashPassword = func(string) (string, error) { return "hash", nil }
	if _, err := svc.SaveSecuritySettings(jfsettings.SecuritySettingsUpdate{NewPassword: "a password long enough"}); !errors.Is(err, persistErr) {
		t.Fatalf("SaveSecuritySettings persistence error = %v", err)
	}

	var nilService *Service
	defaultMCP := nilService.GetMCPServerSettings()
	if defaultMCP.Port != jfsettings.DefaultMCPServerPort || defaultMCP.AuthMode != "token" {
		t.Fatalf("nil service MCP defaults = %#v", defaultMCP)
	}
	if _, err := NewService(nil).SaveMCPServerSettings(jfsettings.MCPServerSettingsUpdate{}); !errors.Is(err, ErrMCPServerStoreUnavailable) {
		t.Fatalf("SaveMCPServerSettings without store error = %v", err)
	}
	if _, _, err := NewService(nil).ResetMCPServerToken(); !errors.Is(err, ErrMCPServerStoreUnavailable) {
		t.Fatalf("ResetMCPServerToken without store error = %v", err)
	}
}

func TestServiceCoverageForMCPRollbackAndOtherSaveFailures(t *testing.T) {
	persistErr := errors.New("persist failed")
	rollbackErr := errors.New("rollback failed")
	store := &coverageErrorStore{fakeStore: &fakeStore{mcpServer: jfsettings.MCPServerSettings{
		Port: jfsettings.DefaultMCPServerPort, AuthMode: "none",
	}}}
	svc := NewService(store, WithSideEffects(SideEffects{
		OnMCPServerChanged: func(jfsettings.MCPServerSettings) error { return persistErr },
	}))
	store.mcpErrors = []error{nil, rollbackErr}
	if _, err := svc.SaveMCPServerSettings(jfsettings.MCPServerSettingsUpdate{Enabled: true}); !errors.Is(err, ErrMCPServerRuntimeUpdate) || !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("SaveMCPServerSettings rollback error = %v", err)
	}

	store.mcpErrors = []error{persistErr}
	svc.newMCPToken = func() (string, error) { return "token", nil }
	svc.hashPassword = func(string) (string, error) { return "hash", nil }
	if _, _, err := svc.ResetMCPServerToken(); !errors.Is(err, persistErr) {
		t.Fatalf("ResetMCPServerToken persistence error = %v", err)
	}

	store.pineWorkerErr = persistErr
	if _, err := svc.SavePineWorkerSettings(jfsettings.PineWorkerSettings{}); !errors.Is(err, persistErr) {
		t.Fatalf("SavePineWorkerSettings error = %v", err)
	}
	store.exchangeCalendarErr = persistErr
	if _, err := svc.SaveExchangeCalendarSettings(jfsettings.ExchangeCalendarSettings{}); !errors.Is(err, persistErr) {
		t.Fatalf("SaveExchangeCalendarSettings error = %v", err)
	}
	store.integrationErr = persistErr
	if _, err := svc.SaveIntegration(jfsettings.BrokerIntegration{}); !errors.Is(err, persistErr) {
		t.Fatalf("SaveIntegration error = %v", err)
	}
}

func TestServiceCoverageForSecurityAndMCPFallbacks(t *testing.T) {
	persistErr := errors.New("listener unavailable")
	rollbackErr := errors.New("settings rollback unavailable")
	store := &coverageErrorStore{fakeStore: &fakeStore{mcpServer: jfsettings.MCPServerSettings{AuthMode: "none"}}}
	svc := NewService(store, WithSideEffects(SideEffects{
		OnSecurityChanged: func(jfsettings.SecuritySettings) error { return persistErr },
	}))

	if _, err := svc.SaveSecuritySettings(jfsettings.SecuritySettingsUpdate{WebAccessEnabled: true}); !errors.Is(err, ErrWebAccessPasswordRequired) {
		t.Fatalf("SaveSecuritySettings without password error = %v", err)
	}
	store.securityErrors = []error{nil, rollbackErr}
	svc.hashPassword = func(string) (string, error) { return "new-hash", nil }
	if _, err := svc.SaveSecuritySettings(jfsettings.SecuritySettingsUpdate{
		WebAccessEnabled: true,
		NewPassword:      "a password long enough",
	}); !errors.Is(err, ErrWebAccessRuntimeUpdate) || !strings.Contains(err.Error(), "settings rollback unavailable") {
		t.Fatalf("SaveSecuritySettings rollback error = %v", err)
	}

	result, err := svc.SaveMCPServerSettings(jfsettings.MCPServerSettingsUpdate{Enabled: false})
	if err != nil || result.Port != jfsettings.DefaultMCPServerPort || result.AuthMode != "none" {
		t.Fatalf("SaveMCPServerSettings fallback = %#v, %v", result, err)
	}

	svc.newMCPToken = func() (string, error) { return "", persistErr }
	if _, _, err := svc.ResetMCPServerToken(); !errors.Is(err, persistErr) {
		t.Fatalf("ResetMCPServerToken generator error = %v", err)
	}
	svc.newMCPToken = func() (string, error) { return "new-token", nil }
	svc.hashPassword = func(string) (string, error) { return "", persistErr }
	if _, _, err := svc.ResetMCPServerToken(); !errors.Is(err, persistErr) {
		t.Fatalf("ResetMCPServerToken hash error = %v", err)
	}

	if _, err := (&Service{}).saveMCPServerSettingsLocked(jfsettings.MCPServerSettings{}, jfsettings.MCPServerSettings{}); !errors.Is(err, ErrMCPServerStoreUnavailable) {
		t.Fatalf("saveMCPServerSettingsLocked without store error = %v", err)
	}
}
