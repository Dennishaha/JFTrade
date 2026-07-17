package definition

import "testing"

func TestValidateScriptAcceptsPineV6Source(t *testing.T) {
	if err := ValidateScript(SourceFormatPineV6, `//@version=6
strategy("Coverage")
strategy.entry("long", strategy.long)`); err != nil {
		t.Fatalf("ValidateScript(valid Pine v6) error = %v", err)
	}
}
