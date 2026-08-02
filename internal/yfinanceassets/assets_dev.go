//go:build !release_assets

package yfinanceassets

import "io/fs"

func assetFS() fs.FS {
	return emptyAssetFS{}
}

// DevelopmentOverridesAllowed reports whether external helper commands may be
// supplied by the local development environment, including Python source
// runtimes selected by runtime dependency settings.
func DevelopmentOverridesAllowed() bool {
	return true
}
