package pineworkerassets

import "testing"

func TestBundleNameIsPlatformIndependent(t *testing.T) {
	if BundleName() != "worker.js" {
		t.Fatalf("BundleName() = %q, want worker.js", BundleName())
	}
}
