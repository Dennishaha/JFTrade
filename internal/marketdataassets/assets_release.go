//go:build release_assets

package marketdataassets

import (
	"embed"
	"io/fs"
)

var (
	//go:embed all:assets/bin
	embeddedAssets embed.FS
)

func assetFS() fs.FS {
	assets, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		return emptyAssetFS{}
	}
	return assets
}

// DevelopmentOverridesAllowed is false for packaged release-assets builds.
func DevelopmentOverridesAllowed() bool {
	return false
}
