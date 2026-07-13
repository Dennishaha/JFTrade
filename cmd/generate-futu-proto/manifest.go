package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func verifyFutuInputs(sourceDirectory, manifestPath string, expectedFiles []string) error {
	checksums, err := parseChecksumManifest(manifestPath)
	if err != nil {
		return err
	}
	if err := validateManifestFiles(checksums, expectedFiles); err != nil {
		return err
	}
	for _, filename := range expectedFiles {
		path := filepath.Join(sourceDirectory, filename)
		content, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("missing proto file: %s", path)
			}
			return fmt.Errorf("read proto file %s: %w", path, err)
		}
		actual := sha256.Sum256(content)
		actualDigest := hex.EncodeToString(actual[:])
		if actualDigest != checksums[filename] {
			return fmt.Errorf(
				"futu proto checksum mismatch for %s: expected %s, got %s",
				path, checksums[filename], actualDigest,
			)
		}
	}
	return nil
}

func parseChecksumManifest(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("futu proto checksum manifest not found: %s", path)
		}
		return nil, fmt.Errorf("open checksum manifest %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	checksums := make(map[string]string)
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || !isLowerHexDigest(fields[0]) {
			return nil, fmt.Errorf("invalid checksum manifest entry at %s:%d", path, lineNumber)
		}
		filename := fields[1]
		if invalidManifestFilename(filename) {
			return nil, fmt.Errorf("invalid checksum manifest filename at %s:%d", path, lineNumber)
		}
		if _, exists := checksums[filename]; exists {
			return nil, fmt.Errorf("invalid checksum manifest filename at %s:%d", path, lineNumber)
		}
		checksums[filename] = fields[0]
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read checksum manifest %s: %w", path, err)
	}
	return checksums, nil
}

func isLowerHexDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func invalidManifestFilename(filename string) bool {
	return filename == "" || strings.ContainsAny(filename, `/\\`) || filepath.Base(filename) != filename
}

func validateManifestFiles(checksums map[string]string, expectedFiles []string) error {
	expected := make(map[string]struct{}, len(expectedFiles))
	for _, filename := range expectedFiles {
		expected[filename] = struct{}{}
	}
	var missing []string
	for _, filename := range expectedFiles {
		if _, found := checksums[filename]; !found {
			missing = append(missing, filename)
		}
	}
	var unexpected []string
	for filename := range checksums {
		if _, found := expected[filename]; !found {
			unexpected = append(unexpected, filename)
		}
	}
	sort.Strings(missing)
	sort.Strings(unexpected)
	if len(missing) == 0 && len(unexpected) == 0 {
		return nil
	}
	var details []string
	if len(missing) != 0 {
		details = append(details, "missing: "+strings.Join(missing, ", "))
	}
	if len(unexpected) != 0 {
		details = append(details, "unexpected: "+strings.Join(unexpected, ", "))
	}
	return fmt.Errorf("futu proto checksum manifest does not match input list (%s)", strings.Join(details, "; "))
}
