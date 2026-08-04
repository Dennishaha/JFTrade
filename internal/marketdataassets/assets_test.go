package marketdataassets

import (
	"runtime"
	"testing"
)

func TestBinaryNameUsesCurrentRuntime(t *testing.T) {
	if BinaryName() != BinaryNameFor(runtime.GOOS, runtime.GOARCH) {
		t.Fatalf("BinaryName() = %q, want runtime-specific name", BinaryName())
	}
}
