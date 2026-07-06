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
	srcDir, dstPath, err := parseArchiveFlags()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	archiveFile, err := os.Create(dstPath)
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

	if err := archiveFrontendAssets(srcDir, zipWriter); err != nil {
		return fmt.Errorf("archive frontend assets: %w", err)
	}
	return nil
}

func parseArchiveFlags() (string, string, error) {
	srcDir := flag.String("src", "", "source directory to archive")
	dstPath := flag.String("dst", "", "destination zip path")
	flag.Parse()
	if *srcDir == "" {
		return "", "", fmt.Errorf("-src is required")
	}
	if *dstPath == "" {
		return "", "", fmt.Errorf("-dst is required")
	}
	return filepath.Clean(*srcDir), filepath.Clean(*dstPath), nil
}

func archiveFrontendAssets(srcDir string, zipWriter *zip.Writer) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		return archiveFrontendAssetFile(srcDir, path, d, zipWriter)
	})
}

func archiveFrontendAssetFile(srcDir string, path string, d fs.DirEntry, zipWriter *zip.Writer) error {
	info, err := d.Info()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported file type in frontend assets: %s", path)
	}
	relPath, err := filepath.Rel(srcDir, path)
	if err != nil {
		return err
	}
	writer, err := zipWriter.CreateHeader(archiveHeader(info, relPath))
	if err != nil {
		return err
	}
	return copyArchiveFile(writer, path)
}

func archiveHeader(info fs.FileInfo, relPath string) *zip.FileHeader {
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return &zip.FileHeader{
			Name:     filepath.ToSlash(relPath),
			Method:   zip.Deflate,
			Modified: fixedArchiveTimestamp,
		}
	}
	header.Name = filepath.ToSlash(relPath)
	header.Method = zip.Deflate
	header.Modified = fixedArchiveTimestamp
	return header
}

func copyArchiveFile(writer io.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(writer, file); err != nil {
		jftradeLogError(file.Close())
		return err
	}
	return file.Close()
}

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
