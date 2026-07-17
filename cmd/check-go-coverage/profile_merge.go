package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// mergeCoverageProfile collapses duplicate blocks emitted by go test when
// -coverpkg instruments a package for more than one test binary. A block is
// covered when any test binary executed it, so the merged count is the maximum
// observed count rather than the sum.
func mergeCoverageProfile(profilePath string) error {
	input, err := os.Open(profilePath)
	if err != nil {
		return fmt.Errorf("open coverage profile: %w", err)
	}
	modeLine, blocks, err := readCoverageProfile(input)
	if closeErr := input.Close(); closeErr != nil && err == nil {
		err = fmt.Errorf("close coverage profile: %w", closeErr)
	}
	if err != nil {
		return err
	}
	return writeMergedCoverageProfile(profilePath, modeLine, blocks)
}

type coverageProfileBlock struct {
	fileRange string
	numStmt   int
	count     int
}

func readCoverageProfile(input *os.File) (string, map[string]coverageProfileBlock, error) {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", nil, fmt.Errorf("read coverage profile mode: %w", err)
		}
		return "", nil, fmt.Errorf("coverage profile is empty")
	}
	modeLine := strings.TrimSpace(scanner.Text())
	if !strings.HasPrefix(modeLine, "mode: ") || strings.TrimSpace(strings.TrimPrefix(modeLine, "mode: ")) == "" {
		return "", nil, fmt.Errorf("coverage profile is missing mode")
	}

	blocks := make(map[string]coverageProfileBlock)
	lineNumber := 1
	for scanner.Scan() {
		lineNumber++
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			return "", nil, fmt.Errorf("parse coverage profile line %d", lineNumber)
		}
		numStmt, parseErr := strconv.Atoi(fields[1])
		if parseErr != nil || numStmt < 0 {
			return "", nil, fmt.Errorf("parse coverage profile statement count on line %d", lineNumber)
		}
		count, parseErr := strconv.Atoi(fields[2])
		if parseErr != nil || count < 0 {
			return "", nil, fmt.Errorf("parse coverage profile execution count on line %d", lineNumber)
		}
		key := fields[0] + "\x00" + fields[1]
		if previous, exists := blocks[key]; !exists || count > previous.count {
			blocks[key] = coverageProfileBlock{fileRange: fields[0], numStmt: numStmt, count: count}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", nil, fmt.Errorf("read coverage profile: %w", err)
	}
	return modeLine, blocks, nil
}

func writeMergedCoverageProfile(profilePath, modeLine string, blocks map[string]coverageProfileBlock) (returnErr error) {
	keys := make([]string, 0, len(blocks))
	for key := range blocks {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	temporary, err := os.CreateTemp(filepath.Dir(profilePath), filepath.Base(profilePath)+"-merged-*")
	if err != nil {
		return fmt.Errorf("create merged coverage profile: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		if err := os.Remove(temporaryPath); err != nil && !os.IsNotExist(err) && returnErr == nil {
			returnErr = fmt.Errorf("remove merged coverage profile: %w", err)
		}
	}()

	if _, err := fmt.Fprintln(temporary, modeLine); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write merged coverage profile mode: %w", err)
	}
	for _, key := range keys {
		item := blocks[key]
		if _, err := fmt.Fprintf(temporary, "%s %d %d\n", item.fileRange, item.numStmt, item.count); err != nil {
			_ = temporary.Close()
			return fmt.Errorf("write merged coverage profile: %w", err)
		}
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close merged coverage profile: %w", err)
	}
	if err := os.Rename(temporaryPath, profilePath); err != nil {
		return fmt.Errorf("replace coverage profile with merged blocks: %w", err)
	}
	return nil
}
