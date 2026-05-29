//go:build release_assets

package frontendassets

import (
	"archive/zip"
	"bytes"
	"io/fs"

	_ "embed"
)

var (
	//go:embed dist.zip
	embeddedArchive []byte
)

func FileSystem() (fs.FS, bool, error) {
	frontendFS, err := zip.NewReader(bytes.NewReader(embeddedArchive), int64(len(embeddedArchive)))
	if err != nil {
		return nil, false, err
	}
	return frontendFS, true, nil
}
