package datamigration

import (
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type managedBackupFile struct {
	path       string
	databaseID string
	size       int64
	createdAt  time.Time
}

func (m *Manager) backupQuotaBytes(ctx context.Context) int64 {
	sourceBytes := int64(0)
	for _, descriptor := range m.descriptors {
		storageBytes := inspectStorage(ctx, DatabaseStatus{Descriptor: descriptor, Status: "ready"}).TotalBytes
		if storageBytes <= 0 {
			continue
		}
		if sourceBytes > math.MaxInt64-storageBytes {
			return math.MaxInt64
		}
		sourceBytes += storageBytes
	}
	if sourceBytes > math.MaxInt64/2 {
		return math.MaxInt64
	}
	return max(backupQuotaFloorBytes, sourceBytes*2)
}

func (m *Manager) prepareBackupCapacity(backupDir, databaseID string, reserveBytes, quotaBytes int64) error {
	files, err := m.listManagedBackupFiles(backupDir)
	if err != nil {
		return err
	}
	remainingForDatabase := 0
	for _, file := range files {
		if file.databaseID == databaseID {
			remainingForDatabase++
		}
	}
	removed := make(map[string]bool)
	for _, file := range files {
		if file.databaseID != databaseID || remainingForDatabase < backupRetentionPerDB {
			continue
		}
		if err := removeManagedBackup(file); err != nil {
			return err
		}
		removed[file.path] = true
		remainingForDatabase--
	}
	if reserveBytes > quotaBytes {
		return fmt.Errorf("%w: backup requires %d bytes but quota is %d bytes", ErrBackupQuotaExceeded, reserveBytes, quotaBytes)
	}
	totalBytes := int64(0)
	for _, file := range files {
		if !removed[file.path] {
			totalBytes += file.size
		}
	}
	for _, file := range files {
		if totalBytes <= quotaBytes-reserveBytes {
			break
		}
		if removed[file.path] {
			continue
		}
		if err := removeManagedBackup(file); err != nil {
			return err
		}
		removed[file.path] = true
		totalBytes -= file.size
	}
	if totalBytes > quotaBytes-reserveBytes {
		return fmt.Errorf("%w: backup directory uses %d bytes and requires %d more bytes", ErrBackupQuotaExceeded, totalBytes, reserveBytes)
	}
	return nil
}

func (m *Manager) enforceBackupRetention(backupDir, currentPath string, quotaBytes int64) error {
	files, err := m.listManagedBackupFiles(backupDir)
	if err != nil {
		return err
	}
	currentPath = filepath.Clean(currentPath)
	currentDatabaseID := ""
	counts := make(map[string]int)
	for _, file := range files {
		counts[file.databaseID]++
		if filepath.Clean(file.path) == currentPath {
			currentDatabaseID = file.databaseID
			if file.size > quotaBytes {
				return fmt.Errorf("%w: backup is %d bytes but quota is %d bytes", ErrBackupQuotaExceeded, file.size, quotaBytes)
			}
		}
	}
	removed := make(map[string]bool)
	for _, file := range files {
		if file.databaseID != currentDatabaseID || counts[file.databaseID] <= backupRetentionPerDB || filepath.Clean(file.path) == currentPath {
			continue
		}
		if err := removeManagedBackup(file); err != nil {
			return err
		}
		removed[file.path] = true
		counts[file.databaseID]--
	}
	totalBytes := int64(0)
	for _, file := range files {
		if !removed[file.path] {
			totalBytes += file.size
		}
	}
	for _, file := range files {
		if totalBytes <= quotaBytes {
			break
		}
		if removed[file.path] || filepath.Clean(file.path) == currentPath {
			continue
		}
		if err := removeManagedBackup(file); err != nil {
			return err
		}
		removed[file.path] = true
		totalBytes -= file.size
	}
	if totalBytes > quotaBytes {
		return fmt.Errorf("%w: backup directory uses %d bytes", ErrBackupQuotaExceeded, totalBytes)
	}
	return nil
}

func (m *Manager) listManagedBackupFiles(backupDir string) ([]managedBackupFile, error) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil, fmt.Errorf("read database backup directory: %w", err)
	}
	files := make([]managedBackupFile, 0, len(entries))
	for _, entry := range entries {
		databaseID, createdAt, ok := m.parseManagedBackupFilename(entry.Name())
		if !ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("inspect database backup %s: %w", entry.Name(), err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		files = append(files, managedBackupFile{
			path: filepath.Join(backupDir, entry.Name()), databaseID: databaseID, size: info.Size(), createdAt: createdAt,
		})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].createdAt.Equal(files[j].createdAt) {
			return files[i].path < files[j].path
		}
		return files[i].createdAt.Before(files[j].createdAt)
	})
	return files, nil
}

func (m *Manager) parseManagedBackupFilename(filename string) (string, time.Time, bool) {
	for _, descriptor := range m.descriptors {
		prefix := descriptor.ID + "-"
		if !strings.HasPrefix(filename, prefix) || !strings.HasSuffix(filename, ".db") {
			continue
		}
		stem := strings.TrimSuffix(strings.TrimPrefix(filename, prefix), ".db")
		separator := strings.LastIndexByte(stem, '-')
		if separator <= 0 || len(stem)-separator-1 != 8 {
			continue
		}
		if _, err := hex.DecodeString(stem[separator+1:]); err != nil {
			continue
		}
		createdAt, err := time.Parse("20060102T150405.000000000Z", stem[:separator])
		if err == nil {
			return descriptor.ID, createdAt, true
		}
	}
	return "", time.Time{}, false
}

func removeManagedBackup(file managedBackupFile) error {
	if err := os.Remove(file.path); err != nil {
		return fmt.Errorf("remove expired database backup %s: %w", filepath.Base(file.path), err)
	}
	return nil
}
