//go:build release_assets

package pineworkerassets

import (
	"embed"
	"io/fs"
)

var (
	//go:embed assets/bin/*
	embeddedAssets embed.FS
)

func assetFS() fs.FS {
	assets, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		return emptyAssetFS{}
	}
	return assets
}
