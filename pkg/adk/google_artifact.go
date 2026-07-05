package adk

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	adkartifact "google.golang.org/adk/v2/artifact"
	"google.golang.org/genai"
)

const googleADKArtifactUserScopeSession = "user"

type googleADKArtifactService struct {
	db *sqliteconn.DB
}

type googleADKArtifactRecord struct {
	appName    string
	userID     string
	sessionID  string
	fileName   string
	version    int64
	partJSON   []byte
	mimeType   string
	createdAt  string
	updatedAt  string
	part       *genai.Part
	customMeta map[string]any
}

func newGoogleADKArtifactService(path string) (adkartifact.Service, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return adkartifact.InMemoryService(), nil
	}
	db, err := sqliteconn.Open(path)
	if err != nil {
		return nil, err
	}
	service := &googleADKArtifactService{db: db}
	if err := service.init(context.Background()); err != nil {
		jftradeErr := db.Close()
		jftradeLogError(jftradeErr)
		return nil, err
	}
	return service, nil
}

func deriveGoogleADKArtifactPathFromSessionService(service any) string {
	provider, ok := service.(adksessionPathProvider)
	if !ok || provider == nil {
		return ""
	}
	path := strings.TrimSpace(provider.DatabasePath())
	if path == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(path), "adk-artifact.db")
}

type adksessionPathProvider interface {
	DatabasePath() string
}

func CloseArtifactService(service adkartifact.Service) error {
	if closer, ok := service.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

func (s *googleADKArtifactService) init(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("ADK artifact database is unavailable")
	}
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS artifacts (
		app_name TEXT NOT NULL,
		user_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		file_name TEXT NOT NULL,
		version INTEGER NOT NULL,
		part_json TEXT NOT NULL,
		mime_type TEXT NOT NULL,
		custom_metadata_json TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (app_name, user_id, session_id, file_name, version)
	)`)
	return err
}

func (s *googleADKArtifactService) Save(ctx context.Context, req *adkartifact.SaveRequest) (*adkartifact.SaveResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("ADK artifact database is unavailable")
	}
	sessionID := googleADKArtifactSessionID(req.SessionID, req.FileName)
	version := req.Version
	if version <= 0 {
		next, err := s.nextVersion(ctx, req.AppName, req.UserID, sessionID, req.FileName)
		if err != nil {
			return nil, err
		}
		version = next
	}
	partJSON, err := json.Marshal(req.Part)
	if err != nil {
		return nil, fmt.Errorf("marshal ADK artifact part: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO artifacts
		(app_name, user_id, session_id, file_name, version, part_json, mime_type, custom_metadata_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, COALESCE((SELECT created_at FROM artifacts WHERE app_name = ? AND user_id = ? AND session_id = ? AND file_name = ? AND version = ?), ?), ?)`,
		req.AppName, req.UserID, sessionID, req.FileName, version, string(partJSON), googleADKArtifactMimeType(req.Part), "null",
		req.AppName, req.UserID, sessionID, req.FileName, version, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("save ADK artifact: %w", err)
	}
	return &adkartifact.SaveResponse{Version: version}, nil
}

func (s *googleADKArtifactService) Load(ctx context.Context, req *adkartifact.LoadRequest) (*adkartifact.LoadResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}
	record, err := s.loadRecord(ctx, req.AppName, req.UserID, googleADKArtifactSessionID(req.SessionID, req.FileName), req.FileName, req.Version)
	if err != nil {
		return nil, err
	}
	return &adkartifact.LoadResponse{Part: record.part}, nil
}

func (s *googleADKArtifactService) Delete(ctx context.Context, req *adkartifact.DeleteRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("request validation failed: %w", err)
	}
	if s == nil || s.db == nil {
		return fmt.Errorf("ADK artifact database is unavailable")
	}
	sessionID := googleADKArtifactSessionID(req.SessionID, req.FileName)
	if req.Version > 0 {
		_, err := s.db.ExecContext(ctx, `DELETE FROM artifacts
			WHERE app_name = ? AND user_id = ? AND session_id = ? AND file_name = ? AND version = ?`,
			req.AppName, req.UserID, sessionID, req.FileName, req.Version,
		)
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM artifacts
		WHERE app_name = ? AND user_id = ? AND session_id = ? AND file_name = ?`,
		req.AppName, req.UserID, sessionID, req.FileName,
	)
	return err
}

func (s *googleADKArtifactService) List(ctx context.Context, req *adkartifact.ListRequest) (*adkartifact.ListResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("ADK artifact database is unavailable")
	}
	rows, err := s.db.QueryxContext(ctx, `SELECT DISTINCT file_name FROM artifacts
		WHERE app_name = ? AND user_id = ? AND (session_id = ? OR session_id = ?)
		ORDER BY file_name`,
		req.AppName, req.UserID, req.SessionID, googleADKArtifactUserScopeSession,
	)
	if err != nil {
		return nil, err
	}
	defer func() { jftradeLogError(rows.Close()) }()
	files := make([]string, 0)
	for rows.Next() {
		var fileName string
		if err := rows.Scan(&fileName); err != nil {
			return nil, err
		}
		files = append(files, fileName)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(files)
	return &adkartifact.ListResponse{FileNames: files}, nil
}

func (s *googleADKArtifactService) Versions(ctx context.Context, req *adkartifact.VersionsRequest) (*adkartifact.VersionsResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("ADK artifact database is unavailable")
	}
	rows, err := s.db.QueryxContext(ctx, `SELECT version FROM artifacts
		WHERE app_name = ? AND user_id = ? AND session_id = ? AND file_name = ?
		ORDER BY version DESC`,
		req.AppName, req.UserID, googleADKArtifactSessionID(req.SessionID, req.FileName), req.FileName,
	)
	if err != nil {
		return nil, err
	}
	defer func() { jftradeLogError(rows.Close()) }()
	versions := make([]int64, 0)
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
	}
	return &adkartifact.VersionsResponse{Versions: versions}, nil
}

func (s *googleADKArtifactService) GetArtifactVersion(ctx context.Context, req *adkartifact.GetArtifactVersionRequest) (*adkartifact.GetArtifactVersionResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}
	record, err := s.loadRecord(ctx, req.AppName, req.UserID, googleADKArtifactSessionID(req.SessionID, req.FileName), req.FileName, req.Version)
	if err != nil {
		return nil, err
	}
	createdAt, _ := time.Parse(time.RFC3339Nano, record.createdAt)
	return &adkartifact.GetArtifactVersionResponse{
		ArtifactVersion: &adkartifact.ArtifactVersion{
			Version:        record.version,
			CanonicalURI:   fmt.Sprintf("sqlite://adk-artifacts/%s/%s/%s/%s/%d", record.appName, record.userID, record.sessionID, record.fileName, record.version),
			CustomMetadata: record.customMeta,
			CreateTime:     createdAt,
			MimeType:       record.mimeType,
		},
	}, nil
}

func (s *googleADKArtifactService) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *googleADKArtifactService) nextVersion(ctx context.Context, appName string, userID string, sessionID string, fileName string) (int64, error) {
	var current sql.NullInt64
	err := s.db.QueryRowxContext(ctx, `SELECT MAX(version) FROM artifacts
		WHERE app_name = ? AND user_id = ? AND session_id = ? AND file_name = ?`,
		appName, userID, sessionID, fileName,
	).Scan(&current)
	if err != nil {
		return 0, err
	}
	if !current.Valid {
		return 1, nil
	}
	return current.Int64 + 1, nil
}

func (s *googleADKArtifactService) loadRecord(ctx context.Context, appName string, userID string, sessionID string, fileName string, version int64) (googleADKArtifactRecord, error) {
	if s == nil || s.db == nil {
		return googleADKArtifactRecord{}, fmt.Errorf("ADK artifact database is unavailable")
	}
	if version <= 0 {
		latest, err := s.latestVersion(ctx, appName, userID, sessionID, fileName)
		if err != nil {
			return googleADKArtifactRecord{}, err
		}
		version = latest
	}
	var record googleADKArtifactRecord
	var partJSON, customMetadataJSON string
	err := s.db.QueryRowxContext(ctx, `SELECT app_name, user_id, session_id, file_name, version, part_json, mime_type, custom_metadata_json, created_at, updated_at
		FROM artifacts
		WHERE app_name = ? AND user_id = ? AND session_id = ? AND file_name = ? AND version = ?`,
		appName, userID, sessionID, fileName, version,
	).Scan(&record.appName, &record.userID, &record.sessionID, &record.fileName, &record.version, &partJSON, &record.mimeType, &customMetadataJSON, &record.createdAt, &record.updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return googleADKArtifactRecord{}, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
	}
	if err != nil {
		return googleADKArtifactRecord{}, err
	}
	var part genai.Part
	if err := json.Unmarshal([]byte(partJSON), &part); err != nil {
		return googleADKArtifactRecord{}, fmt.Errorf("unmarshal ADK artifact part: %w", err)
	}
	record.part = &part
	record.partJSON = []byte(partJSON)
	if strings.TrimSpace(customMetadataJSON) != "" && customMetadataJSON != "null" {
		if err := json.Unmarshal([]byte(customMetadataJSON), &record.customMeta); err != nil {
			return googleADKArtifactRecord{}, fmt.Errorf("unmarshal ADK artifact metadata: %w", err)
		}
	}
	if record.customMeta == nil {
		record.customMeta = map[string]any{}
	}
	return record, nil
}

func (s *googleADKArtifactService) latestVersion(ctx context.Context, appName string, userID string, sessionID string, fileName string) (int64, error) {
	var version int64
	err := s.db.QueryRowxContext(ctx, `SELECT version FROM artifacts
		WHERE app_name = ? AND user_id = ? AND session_id = ? AND file_name = ?
		ORDER BY version DESC LIMIT 1`,
		appName, userID, sessionID, fileName,
	).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
	}
	return version, err
}

func googleADKArtifactSessionID(sessionID string, fileName string) string {
	if strings.HasPrefix(fileName, "user:") {
		return googleADKArtifactUserScopeSession
	}
	return sessionID
}

func googleADKArtifactMimeType(part *genai.Part) string {
	if part != nil && part.InlineData != nil && strings.TrimSpace(part.InlineData.MIMEType) != "" {
		return part.InlineData.MIMEType
	}
	return "text/plain"
}

var _ adkartifact.Service = (*googleADKArtifactService)(nil)
