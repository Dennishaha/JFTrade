package pineworkerassets

import "testing"

func TestBundleNameIsPlatformIndependent(t *testing.T) {
	if BundleName() != "worker.mjs" {
		t.Fatalf("BundleName() = %q, want worker.mjs", BundleName())
	}
}
