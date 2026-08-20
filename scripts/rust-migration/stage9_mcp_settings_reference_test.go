package rustmigration

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/security/passwordhash"
	srvsettings "github.com/jftrade/jftrade-main/internal/settings"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
)

func TestStage9MCPSettingsWriteReference(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"appearance":{"upColor":"#010203","downColor":"#a0b0c0"}}`), 0o600); err != nil {
		t.Fatalf("seed MCP settings: %v", err)
	}
	store, err := settingsfile.New(settingsPath)
	if err != nil {
		t.Fatalf("open MCP settings: %v", err)
	}
	applied := 0
	service := srvsettings.NewService(store, srvsettings.WithSideEffects(srvsettings.SideEffects{
		OnMCPServerChanged: func(jfsettings.MCPServerSettings) error {
			applied++
			return nil
		},
	}))

	_, invalidPort := service.SaveMCPServerSettings(jfsettings.MCPServerSettingsUpdate{
		Enabled: true, Port: 80, AuthMode: "token",
	})
	_, invalidMode := service.SaveMCPServerSettings(jfsettings.MCPServerSettingsUpdate{
		AuthMode: "basic",
	})
	_, tokenRequired := service.SaveMCPServerSettings(jfsettings.MCPServerSettingsUpdate{
		Enabled: true, Port: jfsettings.DefaultMCPServerPort, AuthMode: "token",
	})
	reset, token, err := service.ResetMCPServerToken()
	if err != nil {
		t.Fatalf("reset MCP token: %v", err)
	}
	storedAfterReset := store.MCPServerSettings()
	verified, verifyErr := passwordhash.Verify(storedAfterReset.TokenHash, token)
	publicJSON, err := json.Marshal(reset)
	if err != nil {
		t.Fatalf("encode public MCP settings: %v", err)
	}
	saved, err := service.SaveMCPServerSettings(jfsettings.MCPServerSettingsUpdate{
		Enabled: true, Port: jfsettings.DefaultMCPServerPort, AuthMode: "token",
	})
	if err != nil {
		t.Fatalf("enable MCP settings: %v", err)
	}
	persisted, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read persisted MCP settings: %v", err)
	}
	var persistedDocument map[string]any
	if err := json.Unmarshal(persisted, &persistedDocument); err != nil {
		t.Fatalf("decode persisted MCP settings: %v", err)
	}
	appearance, _ := persistedDocument["appearance"].(map[string]any)

	original := store.MCPServerSettings()
	failingService := srvsettings.NewService(store, srvsettings.WithSideEffects(srvsettings.SideEffects{
		OnMCPServerChanged: func(jfsettings.MCPServerSettings) error {
			return errors.New("port occupied")
		},
	}))
	_, runtimeErr := failingService.SaveMCPServerSettings(jfsettings.MCPServerSettingsUpdate{
		Enabled: true, Port: 7443, AuthMode: "token",
	})
	rolledBack := store.MCPServerSettings()

	result := map[string]any{
		"version":                    "stage9.mcp-settings-write.v1",
		"invalidPortRejected":        errors.Is(invalidPort, srvsettings.ErrMCPServerPortInvalid),
		"invalidModeRejected":        errors.Is(invalidMode, srvsettings.ErrMCPServerAuthModeInvalid),
		"tokenRequiredRejected":      errors.Is(tokenRequired, srvsettings.ErrMCPServerTokenRequired),
		"tokenHasPrefix":             strings.HasPrefix(token, "jft_mcp_"),
		"tokenLength":                len(token),
		"tokenConfigured":            reset.TokenConfigured,
		"verifierValid":              verifyErr == nil && verified,
		"publicLeaksTokenHash":       strings.Contains(string(publicJSON), "tokenHash"),
		"publicLeaksToken":           strings.Contains(string(publicJSON), token),
		"persistedLeaksToken":        strings.Contains(string(persisted), token),
		"persistedHasArgon2id":       strings.Contains(string(persisted), "$argon2id$v=19$m=65536,t=3,p=1$"),
		"unrelatedSettingsPreserved": appearance["upColor"] == "#010203",
		"savedEnabled":               saved.Enabled,
		"successfulRuntimeApplies":   applied,
		"runtimeFailureMapped":       errors.Is(runtimeErr, srvsettings.ErrMCPServerRuntimeUpdate),
		"runtimeFailureRolledBack":   rolledBack == original,
	}
	output := os.Getenv("JFTRADE_STAGE9_MCP_SETTINGS_WRITE_REFERENCE")
	if output == "" {
		return
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encode MCP settings write reference: %v", err)
	}
	if err := os.WriteFile(output, encoded, 0o600); err != nil {
		t.Fatalf("write MCP settings write reference: %v", err)
	}
}
