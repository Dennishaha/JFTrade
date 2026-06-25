package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	outDir := flag.String("out", filepath.Join("docs", "reference", "generated"), "generated reference output directory")
	flag.Parse()

	cleanOutDir := filepath.Clean(*outDir)
	if err := os.MkdirAll(cleanOutDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	path := filepath.Join(cleanOutDir, "pine-v6-support.md")
	if err := os.WriteFile(path, []byte(strategypinespec.BuildSupportSnapshotMarkdown()), 0o644); err != nil {
		return fmt.Errorf("write Pine support snapshot: %w", err)
	}
	return nil
}
