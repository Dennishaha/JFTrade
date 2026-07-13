package protogen

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyFile copies one regular file while preserving its permission bits.
func CopyFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open source file %s: %w", source, err)
	}
	defer func() { _ = input.Close() }()
	info, err := input.Stat()
	if err != nil {
		return fmt.Errorf("inspect source file %s: %w", source, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source file is not regular: %s", source)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create target directory for %s: %w", target, err)
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create target file %s: %w", target, err)
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return fmt.Errorf("copy %s to %s: %w", source, target, err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close target file %s: %w", target, err)
	}
	return nil
}

// Replacement swaps a prepared directory into a repository target.
type Replacement struct {
	Source string
	Target string
}

// ReplaceDirectories replaces all targets and restores the previous targets
// if any rename fails.
func ReplaceDirectories(replacements []Replacement) error {
	states := make([]replacementState, len(replacements))
	for index, replacement := range replacements {
		state, err := backUpTarget(replacement)
		if err != nil {
			return errors.Join(err, restoreBackups(states[:index]))
		}
		states[index] = state
	}
	for index := range states {
		if err := os.Rename(states[index].source, states[index].target); err != nil {
			return errors.Join(
				fmt.Errorf("replace directory %s: %w", states[index].target, err),
				rollbackReplacements(states, index),
			)
		}
		states[index].installed = true
	}
	return removeBackups(states)
}

type replacementState struct {
	source    string
	target    string
	backup    string
	hadTarget bool
	installed bool
}

func backUpTarget(replacement Replacement) (replacementState, error) {
	state := replacementState{source: replacement.Source, target: replacement.Target}
	info, err := os.Stat(replacement.Source)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = fmt.Errorf("source is not a directory")
		}
		return state, fmt.Errorf("inspect replacement source %s: %w", replacement.Source, err)
	}
	_, err = os.Stat(replacement.Target)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("inspect replacement target %s: %w", replacement.Target, err)
	}
	backup, err := reserveBackupPath(replacement.Target)
	if err != nil {
		return state, err
	}
	if err := os.Rename(replacement.Target, backup); err != nil {
		return state, fmt.Errorf("back up directory %s: %w", replacement.Target, err)
	}
	state.backup = backup
	state.hadTarget = true
	return state, nil
}

func reserveBackupPath(target string) (string, error) {
	backup, err := os.MkdirTemp(filepath.Dir(target), "."+filepath.Base(target)+".backup-*")
	if err != nil {
		return "", fmt.Errorf("reserve backup for %s: %w", target, err)
	}
	if err := os.Remove(backup); err != nil {
		return "", fmt.Errorf("prepare backup for %s: %w", target, err)
	}
	return backup, nil
}

func rollbackReplacements(states []replacementState, failedIndex int) error {
	var rollbackErrors []error
	for index := failedIndex - 1; index >= 0; index-- {
		if states[index].installed {
			if err := os.RemoveAll(states[index].target); err != nil {
				rollbackErrors = append(rollbackErrors, err)
			}
		}
	}
	if err := restoreBackups(states); err != nil {
		rollbackErrors = append(rollbackErrors, err)
	}
	return errors.Join(rollbackErrors...)
}

func restoreBackups(states []replacementState) error {
	var restoreErrors []error
	for index := len(states) - 1; index >= 0; index-- {
		state := states[index]
		if !state.hadTarget {
			continue
		}
		if err := os.Rename(state.backup, state.target); err != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("restore directory %s: %w", state.target, err))
		}
	}
	return errors.Join(restoreErrors...)
}

func removeBackups(states []replacementState) error {
	var cleanupErrors []error
	for _, state := range states {
		if state.hadTarget {
			if err := os.RemoveAll(state.backup); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("remove backup %s: %w", state.backup, err))
			}
		}
	}
	return errors.Join(cleanupErrors...)
}
