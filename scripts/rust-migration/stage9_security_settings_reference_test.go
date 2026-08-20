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

func TestStage9SecuritySettingsWriteReference(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"appearance":{"upColor":"#010203","downColor":"#a0b0c0"}}`), 0o600); err != nil {
		t.Fatalf("seed security settings: %v", err)
	}
	store, err := settingsfile.New(settingsPath)
	if err != nil {
		t.Fatalf("open security settings: %v", err)
	}
	applied := 0
	service := srvsettings.NewService(store, srvsettings.WithSideEffects(srvsettings.SideEffects{
		OnSecurityChanged: func(jfsettings.SecuritySettings) error {
			applied++
			return nil
		},
	}))

	_, invalidPort := service.SaveSecuritySettings(jfsettings.SecuritySettingsUpdate{WebPort: 80})
	_, passwordRequired := service.SaveSecuritySettings(jfsettings.SecuritySettingsUpdate{
		WebAccessEnabled: true, WebPort: jfsettings.DefaultWebAccessPort,
	})
	_, passwordTooShort := service.SaveSecuritySettings(jfsettings.SecuritySettingsUpdate{
		NewPassword: "short",
	})
	_, passwordTooLong := service.SaveSecuritySettings(jfsettings.SecuritySettingsUpdate{
		NewPassword: strings.Repeat("a", 1025),
	})
	password := "a sufficiently long password"
	saved, err := service.SaveSecuritySettings(jfsettings.SecuritySettingsUpdate{
		WebAccessEnabled: true, PublicAccessEnabled: true,
		WebPort: jfsettings.DefaultWebAccessPort, NewPassword: password,
	})
	if err != nil {
		t.Fatalf("save security settings: %v", err)
	}
	stored := store.SecuritySettings()
	verified, verifyErr := passwordhash.Verify(stored.PasswordHash, password)
	publicJSON, err := json.Marshal(saved)
	if err != nil {
		t.Fatalf("encode public security settings: %v", err)
	}
	disabled, err := service.SaveSecuritySettings(jfsettings.SecuritySettingsUpdate{
		PublicAccessEnabled: true,
	})
	if err != nil {
		t.Fatalf("disable security settings: %v", err)
	}
	persisted, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read persisted security settings: %v", err)
	}
	var persistedDocument map[string]any
	if err := json.Unmarshal(persisted, &persistedDocument); err != nil {
		t.Fatalf("decode persisted security settings: %v", err)
	}
	appearance, _ := persistedDocument["appearance"].(map[string]any)

	original := store.SecuritySettings()
	failingService := srvsettings.NewService(store, srvsettings.WithSideEffects(srvsettings.SideEffects{
		OnSecurityChanged: func(jfsettings.SecuritySettings) error {
			return errors.New("port occupied")
		},
	}))
	_, runtimeErr := failingService.SaveSecuritySettings(jfsettings.SecuritySettingsUpdate{
		WebAccessEnabled: true, WebPort: 7443,
	})
	rolledBack := store.SecuritySettings()

	result := map[string]any{
		"version":                    "stage9.security-settings-write.v1",
		"invalidPortRejected":        errors.Is(invalidPort, srvsettings.ErrWebAccessPortInvalid),
		"passwordRequiredRejected":   errors.Is(passwordRequired, srvsettings.ErrWebAccessPasswordRequired),
		"passwordTooShortRejected":   errors.Is(passwordTooShort, srvsettings.ErrWebAccessPasswordTooShort),
		"passwordTooLongRejected":    errors.Is(passwordTooLong, srvsettings.ErrWebAccessPasswordTooLong),
		"savedWebAccessEnabled":      saved.WebAccessEnabled,
		"savedPublicAccessEnabled":   saved.PublicAccessEnabled,
		"passwordConfigured":         saved.PasswordConfigured,
		"verifierValid":              verifyErr == nil && verified,
		"publicLeaksPasswordHash":    strings.Contains(string(publicJSON), "passwordHash"),
		"publicLeaksPassword":        strings.Contains(string(publicJSON), password),
		"persistedLeaksPassword":     strings.Contains(string(persisted), password),
		"persistedHasArgon2id":       strings.Contains(string(persisted), "$argon2id$v=19$m=65536,t=3,p=1$"),
		"unrelatedSettingsPreserved": appearance["upColor"] == "#010203",
		"disabledWebAccess":          !disabled.WebAccessEnabled,
		"disabledPublicAccess":       !disabled.PublicAccessEnabled,
		"successfulRuntimeApplies":   applied,
		"runtimeFailureMapped":       errors.Is(runtimeErr, srvsettings.ErrWebAccessRuntimeUpdate),
		"runtimeFailureRolledBack":   rolledBack == original,
	}
	output := os.Getenv("JFTRADE_STAGE9_SECURITY_SETTINGS_WRITE_REFERENCE")
	if output == "" {
		return
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encode security settings write reference: %v", err)
	}
	if err := os.WriteFile(output, encoded, 0o600); err != nil {
		t.Fatalf("write security settings write reference: %v", err)
	}
}
