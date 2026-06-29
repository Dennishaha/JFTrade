//go:build !release_assets

package pineworkerassets

import "testing"

func TestSelectReturnsUnavailableWhenAssetMissing(t *testing.T) {
	asset, ok, err := Select()
	if err != nil {
		t.Fatalf("Select error = %v", err)
	}
	if ok || asset.Name != "" || len(asset.Data) != 0 || asset.SHA256 != "" {
		t.Fatalf("Select = %#v ok=%v, want unavailable empty asset", asset, ok)
	}
}
