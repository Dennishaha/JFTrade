package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func flattenGeneratedFiles(rawDirectory, nextDirectory string) error {
	return filepath.WalkDir(rawDirectory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			return nil
		}
		destination := filepath.Join(nextDirectory, entry.Name())
		if _, err := os.Stat(destination); err == nil {
			return fmt.Errorf("generated filename collision: %s", entry.Name())
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect generated destination %s: %w", destination, err)
		}
		if err := os.Rename(path, destination); err != nil {
			return fmt.Errorf("move generated file %s: %w", path, err)
		}
		return nil
	})
}

func enforceGeneratedLineLimit(directory string, maximum int) error {
	return filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read generated file %s: %w", path, err)
		}
		lines := bytes.Count(content, []byte{'\n'})
		if lines > maximum {
			return fmt.Errorf("generated file exceeds %d lines: %s (%d)", maximum, path, lines)
		}
		return nil
	})
}
