package hk

import "testing"

func TestHKProfileUsesHongKongTimezoneAndSplitSessions(t *testing.T) {
	if got := Location().String(); got != LocationName {
		t.Fatalf("Location = %q, want %q", got, LocationName)
	}
	if len(RegularWindows) != 2 {
		t.Fatalf("RegularWindows len = %d, want 2", len(RegularWindows))
	}
	if RegularWindows[0] != [2]int{9*60 + 30, 12 * 60} || RegularWindows[1] != [2]int{13 * 60, 16 * 60} {
		t.Fatalf("RegularWindows = %#v", RegularWindows)
	}
}
