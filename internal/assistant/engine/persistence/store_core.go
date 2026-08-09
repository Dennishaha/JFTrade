package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jftrade/jftrade-main/internal/assistant/engine/providers"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
	adksession "google.golang.org/adk/v2/session"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

var ErrBuiltinAgentProtected = jfadkmodel.ErrBuiltinAgentProtected

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
)

const (
	TableProviders          = tableProviders
	TableAgents             = tableAgents
	TableSessions           = tableSessions
	TableRuns               = tableRuns
	TableApprovals          = tableApprovals
	TableSkills             = tableSkills
	TableAudit              = tableAudit
	TableOptimizations      = tableOptimizations
	TableTasks              = tableTasks
	TableMemory             = tableMemory
	TableSessionContexts    = tableSessionContexts
	TableHandoffSegments    = tableHandoffSegments
	TableSessionContextLive = tableSessionContextLive
	TableSessionNotices     = tableSessionNotices
	TableSessionComposer    = tableSessionComposer
	TableWorkflows          = tableWorkflows
	TableWorkflowTriggers   = tableWorkflowTriggers
	TableWorkflowTriggerLog = tableWorkflowTriggerLog
)

// StoreOption customizes StoreCore construction.
type StoreOption func(*storeOptions)

type storeOptions struct {
	runNormalizer                func(jfadkmodel.Run) jfadkmodel.Run
	agentNormalizer              func(jfadkmodel.Agent) jfadkmodel.Agent
	timelineEntryNormalizer      func(jfadkmodel.TimelineEntry) jfadkmodel.TimelineEntry
	workflowDefinitionNormalizer func(jfadkmodel.WorkflowDefinition) jfadkmodel.WorkflowDefinition
	workflowTriggerNormalizer    func(jfadkmodel.WorkflowTrigger) jfadkmodel.WorkflowTrigger
	workflowTriggerLogNormalizer func(jfadkmodel.WorkflowTriggerLog) jfadkmodel.WorkflowTriggerLog
	goalRunPredicate             func(jfadkmodel.Run) bool
	preserveUserGoalPause        func(latest jfadkmodel.Run, candidate jfadkmodel.Run) jfadkmodel.Run
	builtinAgentPolicy           BuiltinAgentPolicy
	runLeaseFromContext          func(context.Context) (RunLease, bool)
}

// BuiltinAgentPolicy injects the root package's builtin agent identity rules
// without creating an import cycle.
type BuiltinAgentPolicy struct {
	IsBuiltinID func(string) bool
	IsPrimaryID func(string) bool
	DefaultID   string
	Template    func(string) (jfadkmodel.AgentWriteRequest, bool)
}

// WithBuiltinAgentPolicy injects the composition root's builtin agent
// identity and template rules.
func WithBuiltinAgentPolicy(policy BuiltinAgentPolicy) StoreOption {
	return func(options *storeOptions) {
		options.builtinAgentPolicy = policy
	}
}

// RunLeaseContextAccessors injects the runtime's lease-from-context accessor
// without creating an import cycle.
type RunLeaseContextAccessors struct {
	FromContext func(context.Context) (RunLease, bool)
}

// WithRunLeaseContextAccessors injects the composition root's run lease
// context accessor.
func WithRunLeaseContextAccessors(accessors RunLeaseContextAccessors) StoreOption {
	return func(options *storeOptions) {
		options.runLeaseFromContext = accessors.FromContext
	}
}

// WithRunNormalizer injects the engine's run normalization rules.
func WithRunNormalizer(normalizer func(jfadkmodel.Run) jfadkmodel.Run) StoreOption {
	return func(options *storeOptions) {
		options.runNormalizer = normalizer
	}
}

// WithAgentNormalizer injects the engine's agent normalization rules.
func WithAgentNormalizer(normalizer func(jfadkmodel.Agent) jfadkmodel.Agent) StoreOption {
	return func(options *storeOptions) {
		options.agentNormalizer = normalizer
	}
}

// WithTimelineEntryNormalizer injects the engine's timeline normalization.
func WithTimelineEntryNormalizer(normalizer func(jfadkmodel.TimelineEntry) jfadkmodel.TimelineEntry) StoreOption {
	return func(options *storeOptions) {
		options.timelineEntryNormalizer = normalizer
	}
}

// WithWorkflowDefinitionNormalizer injects workflow definition normalization.
func WithWorkflowDefinitionNormalizer(normalizer func(jfadkmodel.WorkflowDefinition) jfadkmodel.WorkflowDefinition) StoreOption {
	return func(options *storeOptions) {
		options.workflowDefinitionNormalizer = normalizer
	}
}

// WithWorkflowTriggerNormalizer injects workflow trigger normalization.
func WithWorkflowTriggerNormalizer(normalizer func(jfadkmodel.WorkflowTrigger) jfadkmodel.WorkflowTrigger) StoreOption {
	return func(options *storeOptions) {
		options.workflowTriggerNormalizer = normalizer
	}
}

// WithWorkflowTriggerLogNormalizer injects workflow trigger log normalization.
func WithWorkflowTriggerLogNormalizer(normalizer func(jfadkmodel.WorkflowTriggerLog) jfadkmodel.WorkflowTriggerLog) StoreOption {
	return func(options *storeOptions) {
		options.workflowTriggerLogNormalizer = normalizer
	}
}

// WithGoalRunPredicate injects the engine's root goal run predicate.
func WithGoalRunPredicate(predicate func(jfadkmodel.Run) bool) StoreOption {
	return func(options *storeOptions) {
		options.goalRunPredicate = predicate
	}
}

// WithPreserveUserGoalPause injects the engine's user goal pause lifecycle
// preservation rules.
func WithPreserveUserGoalPause(preserve func(latest jfadkmodel.Run, candidate jfadkmodel.Run) jfadkmodel.Run) StoreOption {
	return func(options *storeOptions) {
		options.preserveUserGoalPause = preserve
	}
}

// StoreCore owns the ADK SQLite database, secrets file and session service
// used by the engine composition root.
type StoreCore struct {
	mu         sync.RWMutex
	db         *sqliteconn.DB
	dbPath     string
	secrets    secretStore
	skillsPath string
	sessions   adksession.Service
	opts       storeOptions
	*ClaimStore
}

// NewStoreCore opens the ADK SQLite store and validates the schema. Builtin
// skills and default agents are installed by the composition root.
func NewStoreCore(dbPath string, secretsPath string, skillsPath string, options ...StoreOption) (*StoreCore, error) {
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
	if err := sqliteschema.ValidateCurrentFile(context.Background(), dbPath, sqliteschema.DatabaseADK); err != nil {
		return nil, fmt.Errorf("validate ADK sqlite store: %w", err)
	}
	db, err := sqliteconn.OpenX(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open adk sqlite store: %w", err)
	}
	store := &StoreCore{
		db: db, dbPath: dbPath,
		secrets: secretStore{path: secretsPath}, skillsPath: skillsPath,
		ClaimStore: NewClaimStore(db),
	}
	opts := storeOptions{}
	for _, apply := range options {
		apply(&opts)
	}
	store.opts = opts
	if err := store.initializeOrValidateSchema(); err != nil {
		jftradeErr2 := db.Close()
		besteffort.LogError(jftradeErr2)
		return nil, err
	}
	return store, nil
}

func (s *StoreCore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *StoreCore) SkillsPath() string {
	if s == nil {
		return ""
	}
	return s.skillsPath
}

// DB exposes the underlying SQLite handle for infrastructure and test
// injection. Callers must not retain it across store lifecycle boundaries.
func (s *StoreCore) DB() *sqliteconn.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// SessionService returns the currently configured ADK session service.
func (s *StoreCore) SessionService() adksession.Service {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions
}

// SecretsPath returns the secrets file path used by the store.
func (s *StoreCore) SecretsPath() string {
	if s == nil {
		return ""
	}
	return s.secrets.path
}

// SecretHas reports whether the secrets file contains a non-blank value for
// id.
func (s *StoreCore) SecretHas(id string) bool {
	if s == nil {
		return false
	}
	return s.secrets.has(id)
}

// SecretGet returns the stored secret for id.
func (s *StoreCore) SecretGet(id string) (string, bool, error) {
	if s == nil {
		return "", false, nil
	}
	return s.secrets.get(id)
}

// SecretSet stores the secret for id.
func (s *StoreCore) SecretSet(id string, value string) error {
	if s == nil {
		return nil
	}
	return s.secrets.set(id, value)
}

// SecretDelete removes the stored secret for id.
func (s *StoreCore) SecretDelete(id string) error {
	if s == nil {
		return nil
	}
	return s.secrets.delete(id)
}

// SetSecretsPath relocates the secrets file. It is used by configuration and
// failure-injection paths that must point the store at a fresh file.
func (s *StoreCore) SetSecretsPath(path string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets.path = path
}

func (s *StoreCore) SetSessionService(service adksession.Service) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = service
}

func (s *StoreCore) normalizeRun(run jfadkmodel.Run) jfadkmodel.Run {
	if s.opts.runNormalizer != nil {
		return s.opts.runNormalizer(run)
	}
	return run
}

func (s *StoreCore) normalizeAgent(agent jfadkmodel.Agent) jfadkmodel.Agent {
	if s.opts.agentNormalizer != nil {
		return s.opts.agentNormalizer(agent)
	}
	return agent
}

func (s *StoreCore) normalizeTimelineEntry(entry jfadkmodel.TimelineEntry) jfadkmodel.TimelineEntry {
	if s.opts.timelineEntryNormalizer != nil {
		return s.opts.timelineEntryNormalizer(entry)
	}
	return entry
}

func (s *StoreCore) normalizeWorkflowDefinition(workflow jfadkmodel.WorkflowDefinition) jfadkmodel.WorkflowDefinition {
	if s.opts.workflowDefinitionNormalizer != nil {
		return s.opts.workflowDefinitionNormalizer(workflow)
	}
	return workflow
}

func (s *StoreCore) normalizeWorkflowTrigger(trigger jfadkmodel.WorkflowTrigger) jfadkmodel.WorkflowTrigger {
	if s.opts.workflowTriggerNormalizer != nil {
		return s.opts.workflowTriggerNormalizer(trigger)
	}
	return trigger
}

func (s *StoreCore) normalizeWorkflowTriggerLog(log jfadkmodel.WorkflowTriggerLog) jfadkmodel.WorkflowTriggerLog {
	if s.opts.workflowTriggerLogNormalizer != nil {
		return s.opts.workflowTriggerLogNormalizer(log)
	}
	return log
}

func (s *StoreCore) isBuiltinAgentID(id string) bool {
	return s.opts.builtinAgentPolicy.IsBuiltinID != nil && s.opts.builtinAgentPolicy.IsBuiltinID(id)
}

func (s *StoreCore) isPrimaryBuiltinAgentID(id string) bool {
	return s.opts.builtinAgentPolicy.IsPrimaryID != nil && s.opts.builtinAgentPolicy.IsPrimaryID(id)
}

func (s *StoreCore) defaultAgentID() string {
	return s.opts.builtinAgentPolicy.DefaultID
}

func (s *StoreCore) builtinAgentTemplate(id string) (jfadkmodel.AgentWriteRequest, bool) {
	if s.opts.builtinAgentPolicy.Template == nil {
		return jfadkmodel.AgentWriteRequest{}, false
	}
	return s.opts.builtinAgentPolicy.Template(id)
}

func (s *StoreCore) isRootLoopGoalRun(run jfadkmodel.Run) bool {
	return s.opts.goalRunPredicate != nil && s.opts.goalRunPredicate(run)
}

func (s *StoreCore) preserveUserGoalPauseLifecycle(latest jfadkmodel.Run, candidate jfadkmodel.Run) jfadkmodel.Run {
	if s.opts.preserveUserGoalPause == nil {
		return candidate
	}
	return s.opts.preserveUserGoalPause(latest, candidate)
}

func (s *StoreCore) runLeaseFromContext(ctx context.Context) (RunLease, bool) {
	if s.opts.runLeaseFromContext == nil {
		return RunLease{}, false
	}
	return s.opts.runLeaseFromContext(ctx)
}

func (s *StoreCore) initializeOrValidateSchema() error {
	return sqliteschema.InitializeCurrent(context.Background(), s.db, s.dbPath, sqliteschema.DatabaseADK)
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

// CurrentErrOrNotFound converts a lookup result into os.ErrNotExist when the
// row was not found.
func CurrentErrOrNotFound(err error, ok bool) error {
	return currentErrOrNotFound(err, ok)
}

func (s *StoreCore) listJSON(ctx context.Context, table string, orderBy string, out any) error {
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

// ListJSON reads a JSON payload table into out.
func (s *StoreCore) ListJSON(ctx context.Context, table string, orderBy string, out any) error {
	return s.listJSON(ctx, table, orderBy, out)
}

func (s *StoreCore) listJSONPage(ctx context.Context, table string, clauses []string, args []any, orderBy string, limit int, offset int, out any) (int, error) {
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

// ListJSONPage reads a paged JSON payload table into out.
func (s *StoreCore) ListJSONPage(ctx context.Context, table string, clauses []string, args []any, orderBy string, limit int, offset int, out any) (int, error) {
	return s.listJSONPage(ctx, table, clauses, args, orderBy, limit, offset, out)
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

func (s *StoreCore) getJSON(ctx context.Context, table string, id string, out any) (bool, error) {
	var payload string
	if err := s.db.GetContext(ctx, &payload, `SELECT payload_json FROM `+table+` WHERE id = ?`, strings.TrimSpace(id)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, json.Unmarshal([]byte(payload), out)
}

// GetJSON reads one JSON payload row into out.
func (s *StoreCore) GetJSON(ctx context.Context, table string, id string, out any) (bool, error) {
	return s.getJSON(ctx, table, id, out)
}

func (s *StoreCore) saveJSON(ctx context.Context, table string, id string, createdAt string, updatedAt string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.ExecContext(ctx, `INSERT INTO `+table+` (id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET payload_json = excluded.payload_json, updated_at = excluded.updated_at`, strings.TrimSpace(id), string(payload), createdAt, updatedAt)
	return err
}

// SaveJSON upserts one JSON payload row.
func (s *StoreCore) SaveJSON(ctx context.Context, table string, id string, createdAt string, updatedAt string, value any) error {
	return s.saveJSON(ctx, table, id, createdAt, updatedAt, value)
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

// ValidateProviderBaseURL rejects provider endpoints that are malformed or
// target metadata services.
func ValidateProviderBaseURL(rawURL string) error {
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
	if err := providers.ValidateHostname(parsed.Hostname()); err != nil {
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

// ValidateProviderHeaders rejects provider default headers that could be
// controlled by an upstream proxy or transport layer.
func ValidateProviderHeaders(headers map[string]string) error {
	return validateProviderHeaders(headers)
}
