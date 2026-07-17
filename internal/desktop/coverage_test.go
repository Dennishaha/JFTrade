package desktop

import "testing"

func TestProductDataDirReportsMissingHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	if _, err := ProductDataDir(); err == nil {
		t.Fatal("ProductDataDir() error = nil with user home environment unset")
	}
}

func TestMatchesAnyCoverageForBlankAndNormalizedValues(t *testing.T) {
	if matchesAny("", []string{"error"}) {
		t.Fatal("blank notification value matched")
	}
	if !matchesAny("ERROR", []string{" error "}) {
		t.Fatal("normalized notification value did not match")
	}
}
