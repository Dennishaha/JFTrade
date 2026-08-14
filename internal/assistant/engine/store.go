package adk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	adksession "google.golang.org/adk/v2/session"
)

type DeletedConfigIDs = enginepersistence.DeletedConfigIDs

// Store is the runner-facing composition wrapper around the ADK persistence
// layer. SQL, schema and entity storage live in engine/persistence; this
// package keeps the orchestration-facing methods and construction entry.
type Store struct {
	*enginepersistence.StoreCore
}

// NewStore opens the ADK SQLite store and installs the JFTrade builtin skill
// catalog and default agents.
func NewStore(dbPath string, secretsPath string, skillsPath string) (*Store, error) {
	core, err := enginepersistence.NewStoreCore(
		dbPath,
		secretsPath,
		skillsPath,
		enginepersistence.WithRunNormalizer(NormalizeRun),
		enginepersistence.WithAgentNormalizer(NormalizeAgent),
		enginepersistence.WithTimelineEntryNormalizer(NormalizeTimelineEntry),
		enginepersistence.WithWorkflowDefinitionNormalizer(NormalizeWorkflowDefinition),
		enginepersistence.WithWorkflowTriggerNormalizer(NormalizeWorkflowTrigger),
		enginepersistence.WithWorkflowTriggerLogNormalizer(NormalizeWorkflowTriggerLog),
		enginepersistence.WithGoalRunPredicate(isRootLoopGoalRun),
		enginepersistence.WithPreserveUserGoalPause(preserveUserGoalPauseLifecycle),
		enginepersistence.WithBuiltinAgentPolicy(enginepersistence.BuiltinAgentPolicy{
			IsBuiltinID: IsBuiltinAgentID,
			IsPrimaryID: IsPrimaryBuiltinAgentID,
			DefaultID:   DefaultBuiltinAgentID,
			Template:    BuiltinAgentTemplate,
		}),
		enginepersistence.WithRunLeaseContextAccessors(enginepersistence.RunLeaseContextAccessors{
			FromContext: runExecutionLeaseFromContext,
		}),
	)
	if err != nil {
		return nil, err
	}
	store := &Store{StoreCore: core}
	if err := store.ensureBuiltins(context.Background()); err != nil {
		jftradeErr := core.Close()
		besteffort.LogError(jftradeErr)
		return nil, err
	}
	return store, nil
}

func (s *Store) ensureBuiltins(ctx context.Context) error {
	builtins, err := builtinSkillMetadataCatalog()
	if err != nil {
		return err
	}
	for _, skill := range builtins {
		skill.InstallPath = filepath.Join(s.SkillsPath(), skill.ID, "SKILL.md")
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
		if _, err := s.syncBuiltinAgent(ctx, template); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) syncBuiltinAgent(ctx context.Context, template AgentWriteRequest) (Agent, error) {
	existing, ok, err := s.Agent(ctx, template.ID)
	if err != nil {
		return Agent{}, err
	}
	if !ok {
		return s.SaveAgent(ctx, template)
	}
	// Provider selection and model/reasoning controls remain user-editable. All
	// other primary-builtin fields are policy-owned and are refreshed at startup.
	candidate := NormalizeAgent(Agent{
		ID: template.ID, Name: strings.TrimSpace(template.Name), Instruction: strings.TrimSpace(template.Instruction),
		ProviderID: existing.ProviderID, Model: existing.Model, ReasoningEffort: existing.ReasoningEffort,
		Tools: append([]string(nil), template.Tools...), ToolAccessMode: template.ToolAccessMode,
		Skills: append([]string(nil), template.Skills...), PermissionMode: template.PermissionMode,
		MemoryEnabled: template.MemoryEnabled, RecentUserWindow: template.RecentUserWindow,
		WorkMode: template.WorkMode, LoopMaxIterations: template.LoopMaxIterations,
		Status: template.Status, Builtin: true, CreatedAt: existing.CreatedAt, UpdatedAt: existing.UpdatedAt,
	})
	if reflect.DeepEqual(existing, candidate) {
		return existing, nil
	}
	candidate.UpdatedAt = nowString()
	return candidate, s.SaveJSON(ctx, tableAgents, candidate.ID, candidate.CreatedAt, candidate.UpdatedAt, candidate)
}

func (s *Store) Close() error {
	if s == nil || s.StoreCore == nil {
		return nil
	}
	return s.StoreCore.Close()
}

func (s *Store) SkillsPath() string {
	if s == nil || s.StoreCore == nil {
		return ""
	}
	return s.StoreCore.SkillsPath()
}

func (s *Store) SetSessionService(service adksession.Service) {
	if s == nil || s.StoreCore == nil {
		return
	}
	s.StoreCore.SetSessionService(service)
}

func (s *Store) SessionNotices(ctx context.Context, sessionID string) ([]jfadkmodel.TimelineEntry, error) {
	if s == nil || s.StoreCore == nil {
		return []jfadkmodel.TimelineEntry{}, nil
	}
	return s.StoreCore.SessionNotices(ctx, sessionID)
}

func (s *Store) SaveSessionNotice(ctx context.Context, notice jfadkmodel.TimelineEntry) (jfadkmodel.TimelineEntry, error) {
	if s == nil || s.StoreCore == nil {
		return jfadkmodel.TimelineEntry{}, os.ErrNotExist
	}
	return s.StoreCore.SaveSessionNotice(ctx, notice)
}

func (s *Store) ResolveAndStageApproval(ctx context.Context, approvalID, status string) (jfadkmodel.Approval, bool, *jfadkmodel.Run, bool, error) {
	if s == nil || s.StoreCore == nil {
		return jfadkmodel.Approval{}, false, nil, false, nil
	}
	return s.StoreCore.ResolveAndStageApproval(ctx, approvalID, status)
}

func (s *Store) ResolveRunInput(ctx context.Context, runID string, payload jfadkmodel.InputResponseRequest) (jfadkmodel.Run, bool, error) {
	if s == nil || s.StoreCore == nil {
		return jfadkmodel.Run{}, false, fmt.Errorf("store is unavailable")
	}
	return s.StoreCore.ResolveRunInput(ctx, runID, payload)
}

func (s *Store) PurgeDeletedConfigs(ctx context.Context, ids enginepersistence.DeletedConfigIDs) (int, error) {
	if s == nil || s.StoreCore == nil {
		return 0, fmt.Errorf("adk database is unavailable")
	}
	return s.StoreCore.PurgeDeletedConfigs(ctx, ids)
}

func (s *Store) HasDatabaseActivity(ctx context.Context) (bool, error) {
	if s == nil || s.StoreCore == nil {
		return false, nil
	}
	return s.StoreCore.HasDatabaseActivity(ctx)
}

func (s *Store) CompactDatabase(ctx context.Context) error {
	if s == nil || s.StoreCore == nil {
		return fmt.Errorf("adk database is unavailable")
	}
	return s.StoreCore.CompactDatabase(ctx)
}
