//go:build !windows

package exchangecalendar

import "os"

func replaceSnapshotFile(source string, destination string) error {
	return os.Rename(source, destination)
}

func syncSnapshotDirectory(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer func() { _ = handle.Close() }()
	return handle.Sync()
}
