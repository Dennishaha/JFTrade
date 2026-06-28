//go:build !release_assets

package pineworkerassets

import "testing"

func TestSelectForPlatformReturnsUnavailableWhenAssetMissing(t *testing.T) {
	asset, ok, err := SelectForPlatform("linux", "amd64")
	if err != nil {
		t.Fatalf("SelectForPlatform error = %v", err)
	}
	if ok || asset.Name != "" || len(asset.Data) != 0 || asset.SHA256 != "" {
		t.Fatalf("SelectForPlatform = %#v ok=%v, want unavailable empty asset", asset, ok)
	}
}
