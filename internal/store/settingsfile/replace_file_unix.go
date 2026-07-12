//go:build !windows

package settingsfile

import "os"

func replaceFile(source string, target string) error {
	return os.Rename(source, target)
}

func syncSettingsDirectory(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer func() { _ = handle.Close() }()
	return handle.Sync()
}
