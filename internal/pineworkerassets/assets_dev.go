//go:build !release_assets

package pineworkerassets

import "io/fs"

func assetFS() fs.FS {
	return emptyAssetFS{}
}
