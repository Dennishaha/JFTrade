package trading

import "testing"

func jftradeCheckTestError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("cleanup failed: %v", err)
	}
}
