package definition

import (
	"strings"
	"testing"
)

func TestValidateScriptAcceptsPineV6Source(t *testing.T) {
	if err := ValidateScript(SourceFormatPineV6, `//@version=6
strategy("Coverage")
strategy.entry("long", strategy.long)`); err != nil {
		t.Fatalf("ValidateScript(valid Pine v6) error = %v", err)
	}
}

func TestValidateScriptRejectsUnsupportedFormatAndInvalidSource(t *testing.T) {
	tests := []struct {
		name         string
		sourceFormat string
		script       string
		want         string
	}{
		{name: "unsupported format", sourceFormat: "pine-v5", script: `//@version=6
strategy("Coverage")`, want: "unsupported strategy source format: pine-v5"},
		{name: "missing version header", sourceFormat: SourceFormatPineV6, script: "strategy(", want: "pine script requires //@version=6"},
		{name: "empty script", sourceFormat: SourceFormatPineV6, script: "", want: "pine script is required"},
	}
	for _, test := range tests {
		err := ValidateScript(test.sourceFormat, test.script)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("ValidateScript(%s) error = %v, want substring %q", test.name, err, test.want)
		}
	}
}
