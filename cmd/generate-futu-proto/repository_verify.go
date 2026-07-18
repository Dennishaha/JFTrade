package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const generatedDigestFilename = "futu-proto-generated-10.9.6908.digest"

type futuRepositoryDigest struct {
	ProtoDigest     string
	ProtoFiles      int
	GeneratedDigest string
	GeneratedFiles  int
}

func (d futuRepositoryDigest) String() string {
	return fmt.Sprintf(
		"# Deterministic digest of checked-in OpenD 10.9.6908 generated inputs and outputs.\n"+
			"proto %s %d\npb %s %d\n",
		d.ProtoDigest, d.ProtoFiles, d.GeneratedDigest, d.GeneratedFiles,
	)
}

func inspectFutuRepository(repoRoot string) (futuRepositoryDigest, error) {
	protoDigest, protoFiles, err := digestRepositoryTree(
		filepath.Join(repoRoot, "pkg", "futu", "proto"), ".proto",
	)
	if err != nil {
		return futuRepositoryDigest{}, err
	}
	generatedDigest, generatedFiles, err := digestRepositoryTree(
		filepath.Join(repoRoot, "pkg", "futu", "pb"), ".go",
	)
	if err != nil {
		return futuRepositoryDigest{}, err
	}
	if err := verifyRepositoryProtoNames(repoRoot, protoFiles); err != nil {
		return futuRepositoryDigest{}, err
	}
	return futuRepositoryDigest{
		ProtoDigest: protoDigest, ProtoFiles: len(protoFiles),
		GeneratedDigest: generatedDigest, GeneratedFiles: len(generatedFiles),
	}, nil
}

func digestRepositoryTree(root, extension string) (string, []string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unexpected non-regular generated file: %s", path)
		}
		if filepath.Ext(entry.Name()) != extension {
			return fmt.Errorf("unexpected file in generated tree: %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return "", nil, fmt.Errorf("inspect generated tree %s: %w", root, err)
	}
	if len(files) == 0 {
		return "", nil, fmt.Errorf("generated tree contains no %s files: %s", extension, root)
	}
	slices.Sort(files)
	digest := sha256.New()
	for _, relative := range files {
		if err := writeDigestEntry(digest, root, relative); err != nil {
			return "", nil, err
		}
	}
	return hex.EncodeToString(digest.Sum(nil)), files, nil
}

func writeDigestEntry(digest hash.Hash, root, relative string) error {
	_, _ = io.WriteString(digest, relative)
	_, _ = digest.Write([]byte{0})
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return fmt.Errorf("read generated file %s: %w", relative, err)
	}
	_, _ = digest.Write(content)
	_, _ = digest.Write([]byte{0})
	return nil
}

func verifyRepositoryProtoNames(repoRoot string, relativeFiles []string) error {
	manifestPath := filepath.Join(repoRoot, "scripts", "futu-proto-10.9.6908.sha256")
	expected, err := manifestFileNames(manifestPath)
	if err != nil {
		return err
	}
	actual := make([]string, len(relativeFiles))
	for index, relative := range relativeFiles {
		if strings.Contains(relative, "/") {
			return fmt.Errorf("checked-in Futu proto must remain flat: %s", relative)
		}
		actual[index] = relative
	}
	slices.Sort(actual)
	if !slices.Equal(expected, actual) {
		return fmt.Errorf("checked-in Futu proto filenames do not match %s", manifestPath)
	}
	return nil
}

func writeFutuRepositoryDigest(repoRoot string, digest futuRepositoryDigest) error {
	path := filepath.Join(repoRoot, "scripts", generatedDigestFilename)
	if err := os.WriteFile(path, []byte(digest.String()), 0o644); err != nil {
		return fmt.Errorf("write generated repository digest %s: %w", path, err)
	}
	return nil
}

func verifyFutuRepository(repoRoot string, actual futuRepositoryDigest) error {
	path := filepath.Join(repoRoot, "scripts", generatedDigestFilename)
	expected, err := parseFutuRepositoryDigest(path)
	if err != nil {
		return err
	}
	if expected != actual {
		return fmt.Errorf(
			"checked-in OpenD generated outputs do not match %s; run go run ./cmd/generate-futu-proto with the official 10.9.6908 source (expected proto=%s/%d pb=%s/%d, got proto=%s/%d pb=%s/%d)",
			path,
			expected.ProtoDigest, expected.ProtoFiles,
			expected.GeneratedDigest, expected.GeneratedFiles,
			actual.ProtoDigest, actual.ProtoFiles,
			actual.GeneratedDigest, actual.GeneratedFiles,
		)
	}
	return nil
}

func parseFutuRepositoryDigest(path string) (futuRepositoryDigest, error) {
	file, err := os.Open(path)
	if err != nil {
		return futuRepositoryDigest{}, fmt.Errorf("open generated repository digest %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	var result futuRepositoryDigest
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 || (fields[0] != "proto" && fields[0] != "pb") ||
			!isLowerHexDigest(fields[1]) {
			return futuRepositoryDigest{}, fmt.Errorf("invalid generated repository digest entry in %s", path)
		}
		if seen[fields[0]] {
			return futuRepositoryDigest{}, fmt.Errorf("duplicate generated repository digest entry %q in %s", fields[0], path)
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil || count <= 0 {
			return futuRepositoryDigest{}, fmt.Errorf("invalid generated repository file count in %s", path)
		}
		seen[fields[0]] = true
		if fields[0] == "proto" {
			result.ProtoDigest, result.ProtoFiles = fields[1], count
		} else {
			result.GeneratedDigest, result.GeneratedFiles = fields[1], count
		}
	}
	if err := scanner.Err(); err != nil {
		return futuRepositoryDigest{}, fmt.Errorf("read generated repository digest %s: %w", path, err)
	}
	if !seen["proto"] || !seen["pb"] {
		return futuRepositoryDigest{}, fmt.Errorf("generated repository digest %s must contain proto and pb entries", path)
	}
	return result, nil
}
