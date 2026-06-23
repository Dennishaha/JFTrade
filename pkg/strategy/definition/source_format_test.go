package definition

import (
	"strings"
	"testing"
)

func TestNormalizeSourceFormatDefaultsToPineV6(t *testing.T) {
	if got := NormalizeSourceFormat("  "); got != SourceFormatPineV6 {
		t.Fatalf("NormalizeSourceFormat(blank) = %q, want %q", got, SourceFormatPineV6)
	}
	if got := NormalizeSourceFormat(" Pine-V6 "); got != SourceFormatPineV6 {
		t.Fatalf("NormalizeSourceFormat(trim/case) = %q, want %q", got, SourceFormatPineV6)
	}
}

func TestValidateScriptAndSupportsInstantiation(t *testing.T) {
	if !SupportsInstantiation("") {
		t.Fatal("SupportsInstantiation(blank) = false, want true for default Pine")
	}
	if err := ValidateScript("legacy", "strategy('x')"); err == nil || !strings.Contains(err.Error(), "unsupported strategy source format") {
		t.Fatalf("ValidateScript unsupported format error = %v", err)
	}
	if SupportsInstantiation("legacy") {
		t.Fatal("SupportsInstantiation(legacy) = true, want false")
	}
}
