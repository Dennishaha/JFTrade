package pineworkerassets

import "testing"

func TestBinaryNameMapsSupportedPlatforms(t *testing.T) {
	tests := []struct {
		goos   string
		goarch string
		want   string
	}{
		{goos: "darwin", goarch: "arm64", want: "worker-darwin-arm64"},
		{goos: "darwin", goarch: "amd64", want: "worker-darwin-x64"},
		{goos: "linux", goarch: "amd64", want: "worker-linux-x64"},
		{goos: "linux", goarch: "arm64", want: "worker-linux-arm64"},
		{goos: "windows", goarch: "amd64", want: "worker-windows-x64.exe"},
		{goos: "windows", goarch: "arm64", want: "worker-windows-arm64.exe"},
	}
	for _, test := range tests {
		got, err := BinaryName(test.goos, test.goarch)
		if err != nil {
			t.Fatalf("BinaryName(%s/%s) error = %v", test.goos, test.goarch, err)
		}
		if got != test.want {
			t.Fatalf("BinaryName(%s/%s) = %q, want %q", test.goos, test.goarch, got, test.want)
		}
	}
}

func TestBinaryNameRejectsUnsupportedPlatform(t *testing.T) {
	if _, err := BinaryName("plan9", "amd64"); err == nil {
		t.Fatal("BinaryName unsupported platform error = nil")
	}
}
