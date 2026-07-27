package strategy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategycatalog "github.com/jftrade/jftrade-main/internal/strategy/catalog"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

type catalogRepository struct {
	store *runtimeActivityStore
}

var _ strategycatalog.Repository = (*catalogRepository)(nil)

func (r *catalogRepository) Load(ctx context.Context) (strategycatalog.Snapshot, error) {
	if r == nil || r.store == nil || r.store.db == nil {
		return strategycatalog.Snapshot{}, nil
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	snapshot := strategycatalog.Snapshot{
		Plugins:    []strategycatalog.ManagedPlugin{},
		Strategies: []stratsrv.ManagedInstance{},
		Operations: []stratsrv.PluginOperation{},
	}
	var targetDir string
	err := r.store.db.GetContext(
		ctx,
		&targetDir,
		`SELECT value FROM `+strategyCatalogMetaTable+` WHERE key = ?`,
		"target_dir",
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return strategycatalog.Snapshot{}, fmt.Errorf("load strategy catalog target directory: %w", err)
	}
	snapshot.TargetDir = strings.TrimSpace(targetDir)

	pluginRows := []catalogPayloadRow{}
	if err := r.store.db.SelectContext(
		ctx,
		&pluginRows,
		`SELECT payload_json FROM `+strategyCatalogPluginTable+` ORDER BY id ASC`,
	); err != nil {
		return strategycatalog.Snapshot{}, fmt.Errorf("load strategy catalog plugins: %w", err)
	}
	for _, row := range pluginRows {
		var plugin strategycatalog.ManagedPlugin
		if err := json.Unmarshal([]byte(row.PayloadJSON), &plugin); err != nil {
			return strategycatalog.Snapshot{}, fmt.Errorf("decode strategy catalog plugin: %w", err)
		}
		snapshot.Plugins = append(snapshot.Plugins, plugin)
	}

	strategyRows := []catalogPayloadRow{}
	if err := r.store.db.SelectContext(
		ctx,
		&strategyRows,
		`SELECT payload_json FROM `+strategyCatalogStrategyTable+` ORDER BY created_at ASC, id ASC`,
	); err != nil {
		return strategycatalog.Snapshot{}, fmt.Errorf("load strategy catalog instances: %w", err)
	}
	for _, row := range strategyRows {
		var instance stratsrv.ManagedInstance
		if err := json.Unmarshal([]byte(row.PayloadJSON), &instance); err != nil {
			return strategycatalog.Snapshot{}, fmt.Errorf("decode strategy catalog instance: %w", err)
		}
		snapshot.Strategies = append(snapshot.Strategies, instance)
	}

	operationRows := []catalogPayloadRow{}
	if err := r.store.db.SelectContext(
		ctx,
		&operationRows,
		`SELECT payload_json FROM `+strategyCatalogOperationTable+` ORDER BY updated_at DESC, operation_id ASC`,
	); err != nil {
		return strategycatalog.Snapshot{}, fmt.Errorf("load strategy catalog operations: %w", err)
	}
	for _, row := range operationRows {
		var operation stratsrv.PluginOperation
		if err := json.Unmarshal([]byte(row.PayloadJSON), &operation); err != nil {
			return strategycatalog.Snapshot{}, fmt.Errorf("decode strategy catalog operation: %w", err)
		}
		snapshot.Operations = append(snapshot.Operations, operation)
	}
	return snapshot, nil
}

func (r *catalogRepository) Save(ctx context.Context, snapshot strategycatalog.Snapshot) (resultErr error) {
	if r == nil || r.store == nil || r.store.db == nil {
		return nil
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	tx, err := r.store.db.BeginWrite(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if resultErr != nil {
			besteffort.LogError(tx.Rollback())
		}
	}()
	if err := resetCatalogSnapshot(ctx, tx, snapshot.TargetDir); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, plugin := range snapshot.Plugins {
		payload, err := json.Marshal(plugin)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO `+strategyCatalogPluginTable+` (id, payload_json, updated_at) VALUES (?, ?, ?)`,
			strings.TrimSpace(plugin.Descriptor.ID),
			string(payload),
			now,
		); err != nil {
			return err
		}
	}
	for _, instance := range snapshot.Strategies {
		instance.Logs = nil
		instance.AuditEntries = nil
		payload, err := json.Marshal(instance)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO `+strategyCatalogStrategyTable+` (id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			strings.TrimSpace(instance.ID),
			string(payload),
			strings.TrimSpace(instance.CreatedAt),
			now,
		); err != nil {
			return err
		}
	}
	for _, operation := range snapshot.Operations {
		payload, err := json.Marshal(operation)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO `+strategyCatalogOperationTable+` (operation_id, plugin_id, status, updated_at, payload_json) VALUES (?, ?, ?, ?, ?)`,
			strings.TrimSpace(operation.OperationID),
			strings.TrimSpace(operation.PluginID),
			strings.TrimSpace(operation.Status),
			strings.TrimSpace(operation.UpdatedAt),
			string(payload),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func resetCatalogSnapshot(ctx context.Context, tx *sqliteconn.Tx, targetDir string) error {
	for _, table := range []string{
		strategyCatalogMetaTable,
		strategyCatalogPluginTable,
		strategyCatalogStrategyTable,
		strategyCatalogOperationTable,
	} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO `+strategyCatalogMetaTable+` (key, value) VALUES (?, ?)`,
		"target_dir",
		strings.TrimSpace(targetDir),
	)
	return err
}

type catalogPayloadRow struct {
	PayloadJSON string `db:"payload_json"`
}
