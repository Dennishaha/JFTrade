package datamigration

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
)

func (m *Manager) createBackupSnapshot(
	ctx context.Context,
	descriptor Descriptor,
	status string,
	now time.Time,
	protectedPaths ...string,
) (result BackupResult, err error) {
	backupDir := filepath.Join(filepath.Dir(m.settingsPath), "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return BackupResult{}, fmt.Errorf("create database backup directory: %w", err)
	}
	if err := os.Chmod(backupDir, 0o700); err != nil {
		return BackupResult{}, fmt.Errorf("secure database backup directory: %w", err)
	}
	sourceBytes := max(inspectStorage(ctx, DatabaseStatus{Descriptor: descriptor, Status: status}).TotalBytes, 1)
	quotaBytes := m.backupQuotaBytes(ctx)
	if err := m.prepareBackupCapacity(backupDir, descriptor.ID, sourceBytes, quotaBytes, protectedPaths...); err != nil {
		return BackupResult{}, err
	}
	token, err := newPreviewID()
	if err != nil {
		return BackupResult{}, err
	}
	filename := fmt.Sprintf("%s-%s-%s.db", descriptor.ID, now.Format("20060102T150405.000000000Z"), token[:8])
	backupPath := filepath.Join(backupDir, filename)
	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(backupPath)
		}
	}()

	source, err := sqliteconn.OpenReadOnly(descriptor.Path)
	if err != nil {
		return BackupResult{}, fmt.Errorf("open %s database for backup: %w", descriptor.ID, err)
	}
	_, backupErr := source.ExecContext(ctx, `VACUUM INTO ?`, backupPath)
	closeErr := source.Close()
	if backupErr != nil {
		return BackupResult{}, fmt.Errorf("backup %s database: %w", descriptor.ID, backupErr)
	}
	if closeErr != nil {
		return BackupResult{}, fmt.Errorf("close %s backup source: %w", descriptor.ID, closeErr)
	}
	if err := os.Chmod(backupPath, 0o600); err != nil {
		return BackupResult{}, fmt.Errorf("secure %s database backup: %w", descriptor.ID, err)
	}
	if err := verifySQLiteBackup(ctx, backupPath); err != nil {
		return BackupResult{}, fmt.Errorf("verify %s database backup: %w", descriptor.ID, err)
	}
	protectedPaths = append(protectedPaths, backupPath)
	if err := m.enforceBackupRetention(backupDir, backupPath, quotaBytes, protectedPaths...); err != nil {
		return BackupResult{}, err
	}
	info, err := os.Stat(backupPath)
	if err != nil {
		return BackupResult{}, err
	}
	digest, err := fileSHA256(backupPath)
	if err != nil {
		return BackupResult{}, fmt.Errorf("digest %s database backup: %w", descriptor.ID, err)
	}
	complete = true
	return BackupResult{
		DatabaseID: descriptor.ID,
		BackupPath: backupPath,
		SizeBytes:  info.Size(),
		SHA256:     digest,
		CreatedAt:  now.Format(time.RFC3339Nano),
	}, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
