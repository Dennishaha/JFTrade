package backtest

import "testing"

func isolateBacktestHome(tb testing.TB) {
	tb.Helper()
	home := tb.TempDir()
	tb.Setenv("HOME", home)
	tb.Setenv("USERPROFILE", home)
}
