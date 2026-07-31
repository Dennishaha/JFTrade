//go:build !release_assets

package yfinanceassets

import "io/fs"

func assetFS() fs.FS {
	return emptyAssetFS{}
}
