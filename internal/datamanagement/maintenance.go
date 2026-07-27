package datamanagement

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrCleanupCandidatesChanged reports that an owner could not delete the
// exact candidate set approved by the preview. The application boundary maps
// it to the migration manager's stale-preview error.
var ErrCleanupCandidatesChanged = errors.New("cleanup candidates changed")

// CleanupCandidate is the storage-neutral identity passed to a domain-owned
// cleanup target after the migration manager has validated a preview.
type CleanupCandidate struct {
	ID       string
	Category string
}

// BusyChecker reports whether a domain resource is currently unsafe to
// maintain. Implementations must not mutate the resource.
type BusyChecker interface {
	MaintenanceBusyReason(context.Context) string
}

// BusyCheckers composes independent activity owners in declaration order.
// The first non-empty reason wins so callers receive one stable explanation
// without learning which concrete runtime or store produced it.
type BusyCheckers []BusyChecker

func (checkers BusyCheckers) MaintenanceBusyReason(ctx context.Context) string {
	for _, checker := range checkers {
		if checker == nil {
			continue
		}
		if reason := strings.TrimSpace(checker.MaintenanceBusyReason(ctx)); reason != "" {
			return reason
		}
	}
	return ""
}

// CandidatePurger removes the exact, already-previewed candidate set.
type CandidatePurger interface {
	PurgeMaintenanceCandidates(context.Context, []CleanupCandidate) (int, error)
}

// Compactor compacts one domain-owned persistent resource.
type Compactor interface {
	CompactMaintenanceResource(context.Context) error
}

// Target groups only the maintenance capabilities implemented by one
// database. Nil capabilities remain unavailable and fail closed.
type Target struct {
	Busy      BusyChecker
	Purger    CandidatePurger
	Compactor Compactor
}

// MaintenanceRegistry dispatches maintenance without exposing concrete store
// fields to the database migration manager.
type MaintenanceRegistry struct {
	targets map[string]Target
}

func NewMaintenanceRegistry(targets map[string]Target) *MaintenanceRegistry {
	copyOfTargets := make(map[string]Target, len(targets))
	for databaseID, target := range targets {
		if databaseID = strings.TrimSpace(databaseID); databaseID != "" {
			copyOfTargets[databaseID] = target
		}
	}
	return &MaintenanceRegistry{targets: copyOfTargets}
}

func (r *MaintenanceRegistry) BusyReason(ctx context.Context, databaseID string) string {
	target, ok := r.target(databaseID)
	if !ok || target.Busy == nil {
		return ""
	}
	return strings.TrimSpace(target.Busy.MaintenanceBusyReason(ctx))
}

func (r *MaintenanceRegistry) Purge(
	ctx context.Context,
	databaseID string,
	candidates []CleanupCandidate,
) (int, error) {
	target, ok := r.target(databaseID)
	if !ok || target.Purger == nil {
		return 0, fmt.Errorf("cleanup is unsupported for database %q", databaseID)
	}
	return target.Purger.PurgeMaintenanceCandidates(ctx, candidates)
}

func (r *MaintenanceRegistry) Compact(ctx context.Context, databaseID string) error {
	target, ok := r.target(databaseID)
	if !ok || target.Compactor == nil {
		return fmt.Errorf("database compaction is unavailable for %q", databaseID)
	}
	return target.Compactor.CompactMaintenanceResource(ctx)
}

func (r *MaintenanceRegistry) target(databaseID string) (Target, bool) {
	if r == nil {
		return Target{}, false
	}
	target, ok := r.targets[strings.TrimSpace(databaseID)]
	return target, ok
}

type BusyCheckerFunc func(context.Context) string

func (fn BusyCheckerFunc) MaintenanceBusyReason(ctx context.Context) string {
	if fn == nil {
		return ""
	}
	return fn(ctx)
}

type CandidatePurgerFunc func(context.Context, []CleanupCandidate) (int, error)

func (fn CandidatePurgerFunc) PurgeMaintenanceCandidates(
	ctx context.Context,
	candidates []CleanupCandidate,
) (int, error) {
	if fn == nil {
		return 0, fmt.Errorf("database cleanup is unavailable")
	}
	return fn(ctx, candidates)
}

type CompactorFunc func(context.Context) error

func (fn CompactorFunc) CompactMaintenanceResource(ctx context.Context) error {
	if fn == nil {
		return fmt.Errorf("database compaction is unavailable")
	}
	return fn(ctx)
}
