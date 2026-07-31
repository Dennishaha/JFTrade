//go:build !release_assets

package yfinanceassets

import "testing"

func TestSelectReturnsUnavailableWithoutReleaseAssets(t *testing.T) {
	asset, available, err := Select()
	if err != nil {
		t.Fatalf("Select error = %v", err)
	}
	if available || asset.Name != "" || len(asset.Data) != 0 || asset.SHA256 != "" {
		t.Fatalf("Select = %#v available=%v, want unavailable empty asset", asset, available)
	}
	materialized, available, err := Materialize()
	if err != nil {
		t.Fatalf("Materialize error = %v", err)
	}
	if available || materialized != nil {
		t.Fatalf("Materialize = %#v available=%v, want unavailable", materialized, available)
	}
}

func TestReleaseReturnsUnavailableWithoutReleaseAssets(t *testing.T) {
	materialized, available, err := Release()
	if err != nil {
		t.Fatalf("Release error = %v", err)
	}
	if available || materialized != nil {
		t.Fatalf("Release = %#v available=%v, want unavailable", materialized, available)
	}
}
