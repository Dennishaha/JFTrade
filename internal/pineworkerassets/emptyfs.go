package pineworkerassets

import "io/fs"

type emptyAssetFS struct{}

func (emptyAssetFS) Open(string) (fs.File, error) {
	return nil, fs.ErrNotExist
}
