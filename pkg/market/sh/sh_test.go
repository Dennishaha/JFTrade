package sh

import (
	"testing"
	"time"
)

func TestSHProfileUsesChinaMarketAndTimezone(t *testing.T) {
	if ResolvedMarket != "CN" || PreferredPrefix != "SH" {
		t.Fatalf("market metadata = %q/%q", ResolvedMarket, PreferredPrefix)
	}
	if got := Location().String(); got != LocationName {
		t.Fatalf("Location = %q, want %q", got, LocationName)
	}
	if len(RegularWindows) != 2 || RegularWindows[0][0] != 9*60+30 || RegularWindows[1][1] != 15*60 {
		t.Fatalf("RegularWindows = %#v", RegularWindows)
	}
}

func TestLoadLocationFallsBackToUTC(t *testing.T) {
	if got := loadLocation("Invalid/Timezone"); got != time.UTC {
		t.Fatalf("loadLocation fallback = %v", got)
	}
}
