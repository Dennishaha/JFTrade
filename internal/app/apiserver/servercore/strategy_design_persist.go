package servercore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func deriveStrategyDesignPath(settingsPath string) string {
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultStrategyDesignFilename
	}
	return filepath.Join(directory, defaultStrategyDesignFilename)
}

func deriveStrategyDesignDBPath(legacyPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_STRATEGY_RUNTIME_DB")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(legacyPath))
	if directory == "" || directory == "." {
		return defaultStrategyRuntimeDBFilename
	}
	return filepath.Join(directory, defaultStrategyRuntimeDBFilename)
}

func (s *strategyDesignStore) openDB() error {
	trimmedPath := strings.TrimSpace(s.dbPath)
	if trimmedPath == "" {
		return fmt.Errorf("strategy design db path is required")
	}
	directory := filepath.Dir(trimmedPath)
	if directory != "" && directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create strategy design db directory: %w", err)
		}
	}
	db, err := sqlx.Open("sqlite", trimmedPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return fmt.Errorf("open strategy design sqlite store: %w", err)
	}
	s.db = db
	return nil
}

func strategyDesignDefinitionFromRow(row strategyDesignDefinitionRow) (strategyDesignDefinition, error) {
	var visualModel *strategyVisualModel
	if strings.TrimSpace(row.VisualModelJSON) != "" {
		var parsed strategyVisualModel
		if err := json.Unmarshal([]byte(row.VisualModelJSON), &parsed); err != nil {
			return strategyDesignDefinition{}, err
		}
		visualModel = &parsed
	}
	return normalizeStrategyDesignDefinition(strategyDesignDefinition{
		ID:           row.ID,
		Name:         row.Name,
		Version:      row.Version,
		Description:  row.Description,
		Runtime:      row.Runtime,
		SourceFormat: row.SourceFormat,
		Symbol:       row.Symbol,
		Interval:     row.Interval,
		Script:       row.Script,
		VisualModel:  visualModel,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	})
}

func strategyDesignDefinitionRowFromDefinition(definition strategyDesignDefinition) (strategyDesignDefinitionRow, error) {
	visualModelJSON := ""
	if definition.VisualModel != nil {
		data, err := json.Marshal(definition.VisualModel)
		if err != nil {
			return strategyDesignDefinitionRow{}, err
		}
		visualModelJSON = string(data)
	}
	return strategyDesignDefinitionRow{
		ID:              definition.ID,
		Name:            definition.Name,
		Version:         definition.Version,
		Description:     definition.Description,
		Runtime:         definition.Runtime,
		SourceFormat:    definition.SourceFormat,
		Symbol:          definition.Symbol,
		Interval:        definition.Interval,
		Script:          definition.Script,
		VisualModelJSON: visualModelJSON,
		CreatedAt:       definition.CreatedAt,
		UpdatedAt:       definition.UpdatedAt,
	}, nil
}
