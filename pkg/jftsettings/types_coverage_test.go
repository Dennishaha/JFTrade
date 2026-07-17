package jftsettings

import (
	"encoding/json"
	"testing"
)

func TestExchangeCalendarSettingsUnmarshalExplicitEnabledCoverage(t *testing.T) {
	var settings ExchangeCalendarSettings
	if err := json.Unmarshal([]byte(`{"errorNotificationsEnabled":true}`), &settings); err != nil {
		t.Fatalf("Unmarshal explicit enabled setting: %v", err)
	}
	if !settings.ErrorNotificationsEnabled || !settings.ErrorNotificationsEnabledSet() {
		t.Fatalf("explicit enabled setting = %#v", settings)
	}
}

func TestExchangeCalendarSettingsRejectsInvalidFieldValueInsideObject(t *testing.T) {
	var settings ExchangeCalendarSettings
	if err := json.Unmarshal([]byte(`{"errorNotificationsEnabled":"enabled"}`), &settings); err == nil {
		t.Fatal("Unmarshal accepted a non-boolean errorNotificationsEnabled value")
	}
}
