package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"
)

var fixedArchiveTimestamp = time.Unix(0, 0).UTC()

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	srcDir := flag.String("src", "", "source directory to archive")
	dstPath := flag.String("dst", "", "destination zip path")
	flag.Parse()

	if *srcDir == "" {
		return fmt.Errorf("-src is required")
	}
	if *dstPath == "" {
		return fmt.Errorf("-dst is required")
	}

	trimmedSrcDir := filepath.Clean(*srcDir)
	trimmedDstPath := filepath.Clean(*dstPath)
	if err := os.MkdirAll(filepath.Dir(trimmedDstPath), 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	archiveFile, err := os.Create(trimmedDstPath)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer func() {
		jftradeErr2 := archiveFile.Close()
		jftradeLogError(jftradeErr2)
	}()

	zipWriter := zip.NewWriter(archiveFile)
	defer func() {
		jftradeErr1 := zipWriter.Close()
		jftradeLogError(jftradeErr1)
	}()

	if err := filepath.WalkDir(trimmedSrcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file type in frontend assets: %s", path)
		}

		relPath, err := filepath.Rel(trimmedSrcDir, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath)
		header.Method = zip.Deflate
		header.Modified = fixedArchiveTimestamp

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { jftradeLogError(file.Close()) }()

		if _, err := io.Copy(writer, file); err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("archive frontend assets: %w", err)
	}

	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("close archive writer: %w", err)
	}
	if err := archiveFile.Close(); err != nil {
		return fmt.Errorf("close archive file: %w", err)
	}
	return nil
}

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
