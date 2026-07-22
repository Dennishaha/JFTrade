package adk

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
	adksession "google.golang.org/adk/v2/session"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

var ErrBuiltinAgentProtected = errors.New("builtin agent is protected")

const (
	tableProviders          = "adk_providers"
	tableAgents             = "adk_agents"
	tableSessions           = "adk_sessions"
	tableRuns               = "adk_runs"
	tableApprovals          = "adk_approvals"
	tableSkills             = "adk_skills"
	tableAudit              = "adk_audit_events"
	tableOptimizations      = "adk_optimization_tasks"
	tableTasks              = "adk_tasks"
	tableMemory             = "adk_memory"
	tableSessionContexts    = "adk_session_contexts"
	tableHandoffSegments    = "adk_handoff_segments"
	tableSessionContextLive = "adk_session_context_state"
	tableSessionNotices     = "adk_session_notices"
	tableSessionComposer    = "adk_session_composer_state"
	tableWorkflows          = "adk_workflows"
	tableWorkflowTriggers   = "adk_workflow_triggers"
	tableWorkflowTriggerLog = "adk_workflow_trigger_logs"
	tableRunLeases          = "adk_run_leases"
	tableToolInvocations    = "adk_tool_invocations"
)

type Store struct {
	mu         sync.RWMutex
	db         *sqliteconn.DB
	dbPath     string
	secrets    secretStore
	skillsPath string
	sessions   adksession.Service
}

func NewStore(dbPath string, secretsPath string, skillsPath string) (*Store, error) {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" {
		return nil, fmt.Errorf("adk db path is required")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create adk db directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(secretsPath), 0o700); err != nil {
		return nil, fmt.Errorf("create adk secret directory: %w", err)
	}
	if err := os.MkdirAll(skillsPath, 0o755); err != nil {
		return nil, fmt.Errorf("create adk skills directory: %w", err)
	}
	db, err := sqliteconn.OpenX(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open adk sqlite store: %w", err)
	}
	store := &Store{db: db, dbPath: dbPath, secrets: secretStore{path: secretsPath}, skillsPath: skillsPath}
	if err := store.initializeOrValidateSchema(); err != nil {
		jftradeErr2 := db.Close()
		besteffort.LogError(jftradeErr2)
		return nil, err
	}
	if err := store.ensureBuiltins(context.Background()); err != nil {
		jftradeErr1 := db.Close()
		besteffort.LogError(jftradeErr1)
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) SkillsPath() string {
	if s == nil {
		return ""
	}
	return s.skillsPath
}

func (s *Store) SetSessionService(service adksession.Service) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = service
}

func (s *Store) initializeOrValidateSchema() error {
	statements := []string{
		`CREATE TABLE ` + tableProviders + ` (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE ` + tableAgents + ` (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE ` + tableSessions + ` (id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE ` + tableRuns + ` (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, agent_id TEXT NOT NULL, status TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE ` + tableApprovals + ` (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, agent_id TEXT NOT NULL, status TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE ` + tableSkills + ` (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE ` + tableAudit + ` (id TEXT PRIMARY KEY, kind TEXT NOT NULL, subject_id TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE ` + tableOptimizations + ` (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE ` + tableTasks + ` (id TEXT PRIMARY KEY, status TEXT NOT NULL, agent_id TEXT NOT NULL, run_id TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE ` + tableMemory + ` (id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, scope TEXT NOT NULL, memory_key TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE ` + tableSessionContexts + ` (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE ` + tableHandoffSegments + ` (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, active INTEGER NOT NULL, sequence_no INTEGER NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, payload_json TEXT NOT NULL)`,
		`CREATE TABLE ` + tableSessionContextLive + ` (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE ` + tableSessionNotices + ` (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, run_id TEXT NOT NULL, kind TEXT NOT NULL, status TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE ` + tableSessionComposer + ` (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE INDEX idx_adk_sessions_agent ON ` + tableSessions + ` (agent_id, updated_at DESC)`,
		`CREATE INDEX idx_adk_runs_session ON ` + tableRuns + ` (session_id, created_at DESC)`,
		`CREATE INDEX idx_adk_approvals_status ON ` + tableApprovals + ` (status, updated_at DESC)`,
		`CREATE UNIQUE INDEX idx_adk_approvals_confirmation_call ON ` + tableApprovals + ` (json_extract(payload_json, '$.confirmationCallId')) WHERE COALESCE(json_extract(payload_json, '$.confirmationCallId'), '') <> ''`,
		`CREATE INDEX idx_adk_audit_kind ON ` + tableAudit + ` (kind, created_at DESC)`,
		`CREATE INDEX idx_adk_tasks_status ON ` + tableTasks + ` (status, updated_at DESC)`,
		`CREATE INDEX idx_adk_tasks_agent ON ` + tableTasks + ` (agent_id, updated_at DESC)`,
		`CREATE UNIQUE INDEX idx_adk_memory_agent_scope_key ON ` + tableMemory + ` (agent_id, scope, memory_key)`,
		`CREATE INDEX idx_adk_session_contexts_updated ON ` + tableSessionContexts + ` (updated_at DESC)`,
		`CREATE INDEX idx_adk_handoff_segments_session ON ` + tableHandoffSegments + ` (session_id, sequence_no ASC)`,
		`CREATE INDEX idx_adk_session_context_state_updated ON ` + tableSessionContextLive + ` (updated_at DESC)`,
		`CREATE INDEX idx_adk_session_notices_session ON ` + tableSessionNotices + ` (session_id, created_at ASC)`,
	}
	if err := sqliteschema.InitializeOrValidate(
		context.Background(), s.db, s.dbPath, "adk", 1, statements,
		func(ctx context.Context, db sqliteschema.Database) error {
			for _, schema := range []struct {
				table   string
				columns []string
			}{
				{tableProviders, []string{"id:TEXT:1", "payload_json:TEXT:0", "created_at:TEXT:0", "updated_at:TEXT:0"}},
				{tableAgents, []string{"id:TEXT:1", "payload_json:TEXT:0", "created_at:TEXT:0", "updated_at:TEXT:0"}},
				{tableSessions, []string{"id:TEXT:1", "agent_id:TEXT:0", "payload_json:TEXT:0", "created_at:TEXT:0", "updated_at:TEXT:0"}},
				{tableRuns, []string{"id:TEXT:1", "session_id:TEXT:0", "agent_id:TEXT:0", "status:TEXT:0", "payload_json:TEXT:0", "created_at:TEXT:0", "updated_at:TEXT:0"}},
				{tableApprovals, []string{"id:TEXT:1", "run_id:TEXT:0", "agent_id:TEXT:0", "status:TEXT:0", "payload_json:TEXT:0", "created_at:TEXT:0", "updated_at:TEXT:0"}},
				{tableTasks, []string{"id:TEXT:1", "status:TEXT:0", "agent_id:TEXT:0", "run_id:TEXT:0", "payload_json:TEXT:0", "created_at:TEXT:0", "updated_at:TEXT:0"}},
			} {
				if err := sqliteschema.ValidateTable(ctx, db, schema.table, schema.columns); err != nil {
					return err
				}
			}
			return nil
		},
	); err != nil {
		return err
	}
	if err := s.ensureWorkflowSchema(context.Background()); err != nil {
		return err
	}
	return s.ensureExecutionClaimSchema(context.Background())
}

func (s *Store) ensureBuiltins(ctx context.Context) error {
	if err := s.deleteLegacyBuiltinSkills(ctx); err != nil {
		return err
	}
	builtins, err := builtinSkillMetadataCatalog()
	if err != nil {
		return err
	}
	for _, skill := range builtins {
		skill.InstallPath = filepath.Join(s.skillsPath, skill.ID, "SKILL.md")
		existing, ok, err := s.Skill(ctx, skill.ID)
		if err != nil {
			return err
		}
		if ok {
			skill.Enabled = existing.Enabled
			skill.CreatedAt = existing.CreatedAt
		}
		if _, err := s.SaveSkill(ctx, skill); err != nil {
			return err
		}
	}
	for _, template := range BuiltinAgentTemplates() {
		if _, err := s.EnsureAgent(ctx, template); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) deleteLegacyBuiltinSkills(ctx context.Context) error {
	for _, id := range []string{strategypinespec.LegacyBuiltinSkillName} {
		skill, ok, err := s.Skill(ctx, id)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if !skill.Builtin && !strings.EqualFold(strings.TrimSpace(skill.Source), "builtin") {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM `+tableSkills+` WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

func currentErrOrNotFound(err error, ok bool) error {
	if err != nil {
		return err
	}
	if !ok {
		return os.ErrNotExist
	}
	return nil
}

func (s *Store) listJSON(ctx context.Context, table string, orderBy string, out any) error {
	rows := []struct {
		PayloadJSON string `db:"payload_json"`
	}{}
	query := `SELECT payload_json FROM ` + table
	if orderBy != "" {
		query += ` ORDER BY ` + orderBy
	}
	if err := s.db.SelectContext(ctx, &rows, query); err != nil {
		return err
	}
	bytes, err := json.Marshal(rowsToPayloads(rows))
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, out)
}

func (s *Store) listJSONPage(ctx context.Context, table string, clauses []string, args []any, orderBy string, limit int, offset int, out any) (int, error) {
	whereSQL := ""
	if len(clauses) > 0 {
		whereSQL = " WHERE " + strings.Join(clauses, " AND ")
	}
	countQuery := `SELECT COUNT(*) FROM ` + table + whereSQL
	var total int
	if err := s.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return 0, err
	}
	rows := []struct {
		PayloadJSON string `db:"payload_json"`
	}{}
	query := `SELECT payload_json FROM ` + table + whereSQL
	if orderBy != "" {
		query += ` ORDER BY ` + orderBy
	}
	query += ` LIMIT ? OFFSET ?`
	pageArgs := append(append(make([]any, 0, len(args)+2), args...), limit, offset)
	if err := s.db.SelectContext(ctx, &rows, query, pageArgs...); err != nil {
		return 0, err
	}
	bytes, err := json.Marshal(rowsToPayloads(rows))
	if err != nil {
		return 0, err
	}
	return total, json.Unmarshal(bytes, out)
}

func rowsToPayloads(rows []struct {
	PayloadJSON string `db:"payload_json"`
}) []json.RawMessage {
	payloads := make([]json.RawMessage, 0, len(rows))
	for _, row := range rows {
		payloads = append(payloads, json.RawMessage(row.PayloadJSON))
	}
	return payloads
}

func (s *Store) getJSON(ctx context.Context, table string, id string, out any) (bool, error) {
	var payload string
	if err := s.db.GetContext(ctx, &payload, `SELECT payload_json FROM `+table+` WHERE id = ?`, strings.TrimSpace(id)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, json.Unmarshal([]byte(payload), out)
}

func (s *Store) saveJSON(ctx context.Context, table string, id string, createdAt string, updatedAt string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.ExecContext(ctx, `INSERT INTO `+table+` (id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET payload_json = excluded.payload_json, updated_at = excluded.updated_at`, strings.TrimSpace(id), string(payload), createdAt, updatedAt)
	return err
}

type secretStore struct {
	mu   sync.RWMutex
	path string
}

func (s *secretStore) read() (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readUnlocked()
}

func (s *secretStore) readUnlocked() (map[string]string, error) {
	data := map[string]string{}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return data, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return data, nil
	}
	return data, json.Unmarshal(raw, &data)
}

func (s *secretStore) write(data map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeUnlocked(data)
}

func (s *secretStore) writeUnlocked(data map[string]string) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".adk-secrets-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, s.path)
}

func (s *secretStore) has(id string) bool {
	value, ok, jftradeErr4 := s.get(id)
	besteffort.LogError(jftradeErr4)
	return ok && strings.TrimSpace(value) != ""
}

func (s *secretStore) get(id string) (string, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := s.readUnlocked()
	if err != nil {
		return "", false, err
	}
	value, ok := data[strings.TrimSpace(id)]
	return value, ok, nil
}

func (s *secretStore) set(id string, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.readUnlocked()
	if err != nil {
		return err
	}
	data[strings.TrimSpace(id)] = value
	return s.writeUnlocked(data)
}

func (s *secretStore) delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.readUnlocked()
	if err != nil {
		return err
	}
	delete(data, strings.TrimSpace(id))
	return s.writeUnlocked(data)
}

func normalizeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if ok {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-_")
}

func defaultString(value string, defaultValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	return value
}

func normalizeBaseURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func normalizeProvider(provider Provider) Provider {
	provider.RequestTimeoutMs = normalizeProviderRequestTimeoutMs(provider.RequestTimeoutMs)
	provider.ContextWindowTokens = normalizeContextWindowTokens(provider.ContextWindowTokens)
	return provider
}

func normalizeContextWindowTokens(value int) int {
	if value <= 0 {
		return 0
	}
	if value < 1_024 {
		return 1_024
	}
	if value > 10_000_000 {
		return 10_000_000
	}
	return value
}

func normalizeRecentUserWindow(value int) int {
	switch {
	case value <= 0:
		return 6
	case value < 2:
		return 2
	case value > 100:
		return 100
	default:
		return value
	}
}

func normalizeProviderRequestTimeoutMs(value int) int {
	const (
		minProviderRequestTimeoutMs = 15_000
		maxProviderRequestTimeoutMs = 600_000
	)
	if value <= 0 {
		return int(DefaultProviderRequestTimeout / time.Millisecond)
	}
	if value < minProviderRequestTimeoutMs {
		return minProviderRequestTimeoutMs
	}
	if value > maxProviderRequestTimeoutMs {
		return maxProviderRequestTimeoutMs
	}
	return value
}

func normalizeHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(headers))
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			normalized[key] = value
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizeStringSlice(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizePermissionMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case PermissionModeLessApproval:
		return PermissionModeLessApproval
	case PermissionModeAll:
		return PermissionModeAll
	default:
		return PermissionModeApproval
	}
}

func validPermissionMode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll:
		return true
	default:
		return false
	}
}

func normalizeWorkMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case WorkModeLoop:
		return WorkModeLoop
	default:
		return WorkModeChat
	}
}

func normalizeAgentDefaultWorkMode(value string) string {
	switch normalizeWorkMode(value) {
	case WorkModeLoop:
		return normalizeWorkMode(value)
	default:
		return WorkModeChat
	}
}

func validWorkMode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", WorkModeChat, WorkModeLoop:
		return true
	default:
		return false
	}
}

func normalizeLoopMaxIterations(value int) int {
	switch {
	case value <= 0:
		return DefaultLoopMaxIterations
	case value > MaxLoopIterations:
		return MaxLoopIterations
	default:
		return value
	}
}

func normalizeTaskStatus(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "TODO", "IN_PROGRESS", "BLOCKED", "DONE", "CANCELLED":
		return strings.ToUpper(strings.TrimSpace(value)), nil
	case "":
		return "TODO", nil
	default:
		return "", fmt.Errorf("invalid task status %q", value)
	}
}

func normalizeTaskDependsOn(taskID string, values []string) ([]string, error) {
	taskID = normalizeID(taskID)
	normalized := normalizeStringSlice(values)
	for _, value := range normalized {
		if normalizeID(value) == taskID {
			return nil, fmt.Errorf("task cannot depend on itself")
		}
	}
	return normalized, nil
}

func normalizeMemoryKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if ok {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-_")
}

func defaultAgentInstruction() string {
	return "你是 JFTrade 投资分析 agent。优先使用内部行情、账户、策略和回测工具；涉及安装 skill、保存策略、运行优化或改变自动化状态时遵守当前审批等级。输出必须说明使用了哪些数据来源，不提供保证收益承诺。"
}

func validateProviderBaseURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid provider base URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("provider base URL must use http or https scheme")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return fmt.Errorf("provider base URL must have a host")
	}
	if err := validateProviderHostname(parsed.Hostname()); err != nil {
		return err
	}
	return nil
}

func validateProviderHeaders(headers map[string]string) error {
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		lower := strings.ToLower(key)
		switch lower {
		case "host", "connection", "content-length", "transfer-encoding", "upgrade":
			return fmt.Errorf("provider default header %q is not allowed", key)
		}
		if strings.HasPrefix(lower, "sec-") || strings.HasPrefix(lower, "proxy-") {
			return fmt.Errorf("provider default header %q is not allowed", key)
		}
		_ = value
	}
	return nil
}
