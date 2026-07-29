package jftsettings

import (
	"encoding/json"
	"testing"
)

func TestExchangeCalendarSettingsDefaultsLegacyErrorNotifications(t *testing.T) {
	var settings ExchangeCalendarSettings
	if err := json.Unmarshal([]byte(`{"autoRefreshEnabled":true,"refreshIntervalHours":12}`), &settings); err != nil {
		t.Fatalf("Unmarshal legacy settings: %v", err)
	}
	if !settings.ErrorNotificationsEnabled {
		t.Fatal("legacy settings should default error notifications on")
	}
	if settings.ErrorNotificationsEnabledSet() {
		t.Fatal("legacy settings should not mark error notification as explicitly set")
	}
}

func TestExchangeCalendarSettingsPreservesExplicitErrorNotifications(t *testing.T) {
	var settings ExchangeCalendarSettings
	if err := json.Unmarshal([]byte(`{"errorNotificationsEnabled":false}`), &settings); err != nil {
		t.Fatalf("Unmarshal explicit settings: %v", err)
	}
	if settings.ErrorNotificationsEnabled {
		t.Fatal("explicit false should be preserved")
	}
	if !settings.ErrorNotificationsEnabledSet() {
		t.Fatal("explicit field should be marked as set")
	}

	settings = settings.WithErrorNotificationsEnabledSet(false)
	if settings.ErrorNotificationsEnabledSet() {
		t.Fatal("WithErrorNotificationsEnabledSet(false) did not clear marker")
	}
}

func TestExchangeCalendarSettingsRejectsMalformedJSON(t *testing.T) {
	var settings ExchangeCalendarSettings
	if err := json.Unmarshal([]byte(`{"manualOverrides":`), &settings); err == nil {
		t.Fatal("malformed settings JSON should fail")
	}
}
