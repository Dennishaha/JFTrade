package exchangecalendar

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSourceStatusJSONOmitsZeroTimes(t *testing.T) {
	raw, err := json.Marshal(SourceStatus{SourceID: "nyse_official", Enabled: true})
	if err != nil {
		t.Fatalf("Marshal zero status: %v", err)
	}
	text := string(raw)
	for _, field := range []string{"lastSuccessAt", "lastFailureAt", "nextRefreshAt", "lastSnapshotFetchedAt", "lastProbeAt", "lastProbeSuccessAt", "lastProbeFailureAt", "lastAlertAt"} {
		if strings.Contains(text, `"`+field+`"`) {
			t.Fatalf("zero status JSON = %s, unexpectedly contains %s", text, field)
		}
	}

	at := time.Date(2026, time.June, 23, 9, 30, 0, 0, time.UTC)
	raw, err = json.Marshal(SourceStatus{SourceID: "nyse_official", LastSuccessAt: at})
	if err != nil {
		t.Fatalf("Marshal nonzero status: %v", err)
	}
	if !strings.Contains(string(raw), `"lastSuccessAt":"2026-06-23T09:30:00Z"`) {
		t.Fatalf("nonzero status JSON = %s, missing lastSuccessAt", raw)
	}
}
