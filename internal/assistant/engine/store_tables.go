package adk

import enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"

const (
	tableProviders          = enginepersistence.TableProviders
	tableAgents             = enginepersistence.TableAgents
	tableSessions           = enginepersistence.TableSessions
	tableRuns               = enginepersistence.TableRuns
	tableApprovals          = enginepersistence.TableApprovals
	tableSkills             = enginepersistence.TableSkills
	tableAudit              = enginepersistence.TableAudit
	tableOptimizations      = enginepersistence.TableOptimizations
	tableTasks              = enginepersistence.TableTasks
	tableMemory             = enginepersistence.TableMemory
	tableSessionContexts    = enginepersistence.TableSessionContexts
	tableHandoffSegments    = enginepersistence.TableHandoffSegments
	tableSessionContextLive = enginepersistence.TableSessionContextLive
	tableSessionNotices     = enginepersistence.TableSessionNotices
	tableSessionComposer    = enginepersistence.TableSessionComposer
	tableWorkflows          = enginepersistence.TableWorkflows
	tableWorkflowTriggers   = enginepersistence.TableWorkflowTriggers
	tableWorkflowTriggerLog = enginepersistence.TableWorkflowTriggerLog
	tableRunLeases          = enginepersistence.TableRunLeases
	tableToolInvocations    = enginepersistence.TableToolInvocations
)
