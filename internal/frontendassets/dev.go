//go:build !release_assets

package frontendassets

import "io/fs"

// FileSystem reports whether release frontend assets were embedded.
// Default builds keep frontend assets external so normal development and tests
// do not depend on a prebuilt dist directory.
func FileSystem() (fs.FS, bool, error) {
	return nil, false, nil
}
