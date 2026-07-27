package assembly

import (
	"context"
	"errors"
	"fmt"

	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

type MaintenanceResource string

const (
	MaintenanceRuntimeDatabase  MaintenanceResource = "runtime"
	MaintenanceSessionDatabase  MaintenanceResource = "session"
	MaintenanceArtifactDatabase MaintenanceResource = "artifact"
)

// DatabaseMaintenance adapts one ADK-owned database to the data-management
// ports without exposing Runtime.Store() to application assembly.
type DatabaseMaintenance struct {
	runtime  *jfadk.Runtime
	resource MaintenanceResource
}

func newDatabaseMaintenance(
	runtime *jfadk.Runtime,
	resource MaintenanceResource,
) *DatabaseMaintenance {
	return &DatabaseMaintenance{runtime: runtime, resource: resource}
}

func (m *DatabaseMaintenance) MaintenanceBusyReason(ctx context.Context) string {
	if m == nil || m.runtime == nil {
		return ""
	}
	active, err := m.runtime.HasDatabaseActivity(ctx)
	if err != nil {
		return "无法确认 ADK 运行状态"
	}
	if active {
		return "存在活动、暂停或等待审批的 ADK 运行"
	}
	return ""
}

func (m *DatabaseMaintenance) PurgeMaintenanceCandidates(
	ctx context.Context,
	candidates []dmsrv.CleanupCandidate,
) (int, error) {
	if m == nil || m.runtime == nil || m.resource != MaintenanceRuntimeDatabase {
		return 0, fmt.Errorf("adk database is unavailable")
	}
	ids := jfadk.DeletedConfigIDs{}
	for _, candidate := range candidates {
		switch candidate.Category {
		case "智能体":
			ids.Agents = append(ids.Agents, candidate.ID)
		case "工作流":
			ids.Workflows = append(ids.Workflows, candidate.ID)
		case "触发器":
			ids.Triggers = append(ids.Triggers, candidate.ID)
		}
	}
	deleted, err := m.runtime.PurgeDeletedConfigs(ctx, ids)
	if errors.Is(err, jfadk.ErrCleanupCandidatesChanged) {
		return 0, fmt.Errorf("%w: %v", dmsrv.ErrCleanupCandidatesChanged, err)
	}
	if err != nil {
		return 0, err
	}
	if deleted != len(candidates) {
		return 0, dmsrv.ErrCleanupCandidatesChanged
	}
	return deleted, nil
}

func (m *DatabaseMaintenance) CompactMaintenanceResource(ctx context.Context) error {
	if m == nil || m.runtime == nil {
		return fmt.Errorf("adk %s database is unavailable", maintenanceResourceName(m))
	}
	switch m.resource {
	case MaintenanceRuntimeDatabase:
		return m.runtime.CompactDatabase(ctx)
	case MaintenanceSessionDatabase:
		return m.runtime.CompactSessionDatabase(ctx)
	case MaintenanceArtifactDatabase:
		return m.runtime.CompactArtifactDatabase(ctx)
	default:
		return fmt.Errorf("adk database maintenance resource %q is unsupported", m.resource)
	}
}

func maintenanceResourceName(maintenance *DatabaseMaintenance) string {
	if maintenance == nil {
		return "runtime"
	}
	switch maintenance.resource {
	case MaintenanceSessionDatabase:
		return "session"
	case MaintenanceArtifactDatabase:
		return "artifact"
	default:
		return "runtime"
	}
}
