package servercore

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// removeLegacyAdminKey deletes only the application-owned key file used by
// releases before Web password authentication. Environment-provided paths are
// intentionally ignored so JFTrade never deletes an arbitrary user file.
func removeLegacyAdminKey(settingsPath string) {
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		directory = "."
	}
	path := filepath.Join(directory, "secrets", "admin.key")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("JFTrade could not remove obsolete admin key file %s: %v", path, err)
	}
}
