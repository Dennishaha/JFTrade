package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	protoPackagePattern   = regexp.MustCompile(`(?m)^\s*package\s+([A-Za-z0-9_]+)\s*;`)
	protoGoPackagePattern = regexp.MustCompile(`(?m)^\s*option\s+go_package\s*=.*$`)
	goPackagePattern      = regexp.MustCompile(`(?m)^package\s+([A-Za-z0-9_]+)`)
)

func rewriteGoPackages(stageDirectory string) (int, error) {
	entries, err := os.ReadDir(stageDirectory)
	if err != nil {
		return 0, fmt.Errorf("read staged proto directory: %w", err)
	}
	rewritten := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".proto" {
			continue
		}
		if err := rewriteGoPackage(filepath.Join(stageDirectory, entry.Name())); err != nil {
			return 0, err
		}
		rewritten++
	}
	return rewritten, nil
}

func rewriteGoPackage(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read staged proto %s: %w", path, err)
	}
	content = normalizeNewlines(content)
	packageMatch := protoPackagePattern.FindSubmatchIndex(content)
	if packageMatch == nil {
		return fmt.Errorf("no package in %s", path)
	}
	packageName := string(content[packageMatch[2]:packageMatch[3]])
	goPackageName := strings.ToLower(strings.ReplaceAll(packageName, "_", ""))
	target := "github.com/jftrade/jftrade-main/pkg/futu/pb/" + goPackageName + ";" + goPackageName
	option := []byte(`option go_package = "` + target + `";`)
	optionMatch := protoGoPackagePattern.FindIndex(content)
	var rewritten []byte
	if optionMatch != nil {
		rewritten = replaceRange(content, optionMatch[0], optionMatch[1], option)
	} else {
		insertAt := packageMatch[1]
		rewritten = make([]byte, 0, len(content)+len(option)+1)
		rewritten = append(rewritten, content[:insertAt]...)
		rewritten = append(rewritten, '\n')
		rewritten = append(rewritten, option...)
		rewritten = append(rewritten, content[insertAt:]...)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect staged proto %s: %w", path, err)
	}
	if err := os.WriteFile(path, rewritten, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write staged proto %s: %w", path, err)
	}
	return nil
}

func normalizeNewlines(content []byte) []byte {
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	return []byte(strings.ReplaceAll(normalized, "\r", "\n"))
}

func replaceRange(content []byte, start, end int, replacement []byte) []byte {
	result := make([]byte, 0, len(content)-(end-start)+len(replacement))
	result = append(result, content[:start]...)
	result = append(result, replacement...)
	result = append(result, content[end:]...)
	return result
}

func organizeGeneratedFiles(outputDirectory string) (int, error) {
	entries, err := os.ReadDir(outputDirectory)
	if err != nil {
		return 0, fmt.Errorf("read generated output directory: %w", err)
	}
	moved := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pb.go") {
			continue
		}
		source := filepath.Join(outputDirectory, entry.Name())
		content, err := os.ReadFile(source)
		if err != nil {
			return 0, fmt.Errorf("read generated file %s: %w", source, err)
		}
		match := goPackagePattern.FindSubmatch(content)
		if match == nil {
			continue
		}
		destinationDirectory := filepath.Join(outputDirectory, string(match[1]))
		if err := os.MkdirAll(destinationDirectory, 0o755); err != nil {
			return 0, fmt.Errorf("create generated package directory: %w", err)
		}
		destination := filepath.Join(destinationDirectory, entry.Name())
		if _, err := os.Stat(destination); err == nil {
			return 0, fmt.Errorf("generated destination already exists: %s", destination)
		} else if !os.IsNotExist(err) {
			return 0, fmt.Errorf("inspect generated destination %s: %w", destination, err)
		}
		if err := os.Rename(source, destination); err != nil {
			return 0, fmt.Errorf("move generated file %s: %w", source, err)
		}
		moved++
	}
	return moved, nil
}
