package protogen

import (
	"fmt"
	"os"
	"path/filepath"
)

// FindRepoRoot walks upward from start until it finds go.mod.
func FindRepoRoot(start string) (string, error) {
	directory, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	for {
		info, statErr := os.Stat(filepath.Join(directory, "go.mod"))
		if statErr == nil && !info.IsDir() {
			return directory, nil
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", fmt.Errorf("inspect go.mod: %w", statErr)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", fmt.Errorf("find repository root from %s: go.mod not found", start)
		}
		directory = parent
	}
}
