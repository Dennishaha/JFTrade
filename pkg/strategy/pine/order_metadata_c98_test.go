package pine

import (
	"strings"
	"testing"
)

func TestCoverage98OrderMetadataRejectsAmbiguousInputsAndKeepsSupportedPositionals(t *testing.T) {
	if mode, value := pineCloseQuantity([]string{`id="entry"`, "25"}, "entry"); mode != "symbol_position_percent" || value != "25" {
		t.Fatalf("second positional close quantity = %q/%q", mode, value)
	}
	if err := rejectConflictingQuantityArgs(8, "strategy.close", []string{"qty=1", "qty_percent=20"}); err == nil || !strings.Contains(err.Error(), "qty or qty_percent") {
		t.Fatalf("conflicting close quantity error = %v", err)
	}
	if err := rejectUnsupportedNamedArgs(8, "strategy.close", []string{"unknown=true"}, "qty"); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unsupported named argument error = %v", err)
	}
	if err := rejectUnsupportedOrderArgs(8, "strategy.entry", []string{"oca_type=strategy.oca.cancel"}); err == nil || !strings.Contains(err.Error(), "oca_type") {
		t.Fatalf("unsupported OCA error = %v", err)
	}

	if _, _, _, err := pineOrderMetadata(9, "strategy.entry", []string{"disable_alert=maybe"}, false); err == nil || !strings.Contains(err.Error(), "disable_alert") {
		t.Fatalf("invalid disable_alert error = %v", err)
	}
	if _, _, _, err := pineOrderMetadata(9, "strategy.entry", []string{"immediately=true"}, false); err == nil || !strings.Contains(err.Error(), "immediately") {
		t.Fatalf("unsupported immediate error = %v", err)
	}
	if _, _, _, _, err := pineCloseMetadata(10, "strategy.close", []string{"immediately=maybe"}); err == nil || !strings.Contains(err.Error(), "immediately") {
		t.Fatalf("invalid close immediate error = %v", err)
	}
	if _, _, _, _, err := pineCloseAllMetadata(11, []string{"maybe"}); err == nil || !strings.Contains(err.Error(), "immediately") {
		t.Fatalf("invalid positional close_all immediate error = %v", err)
	}
	if _, _, _, _, err := pineCloseAllMetadata(11, []string{"true", `"comment"`, `"alert"`, "false", "extra"}); err == nil || !strings.Contains(err.Error(), "positional") {
		t.Fatalf("excess close_all positional error = %v", err)
	}
	if _, err := pineExitMetadata(12, []string{"immediately=true"}); err == nil || !strings.Contains(err.Error(), "does not support immediately") {
		t.Fatalf("exit immediate error = %v", err)
	}

	comment, alert, disabled, immediate, err := pineCloseAllMetadata(13, []string{"true", `'close comment'`, `'close alert'`, "true"})
	if err != nil || comment != "close comment" || alert != "close alert" || !disabled || !immediate {
		t.Fatalf("supported close_all positional metadata = %q/%q/%v/%v, %v", comment, alert, disabled, immediate, err)
	}
	if got := unquote(`'single quoted fallback'`); got != "single quoted fallback" {
		t.Fatalf("single quoted fallback = %q", got)
	}
}
