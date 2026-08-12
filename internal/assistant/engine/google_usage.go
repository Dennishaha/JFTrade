package adk

import adksession "google.golang.org/adk/v2/session"

func (e *googleADKExecution) consumeUsage(event *adksession.Event) {
	if e == nil || event == nil {
		return
	}
	e.mu.Lock()
	runID := e.runIDForAgentName(event.Author)
	base := e.runBaseLocked(runID)
	usage, changed := e.usageProjection.Accumulate(event.ID, event.Partial, event.UsageMetadata, base.Usage)
	if !changed {
		e.mu.Unlock()
		return
	}
	base.Usage = usage
	e.runSnapshotBaseByID[runID] = base
	deltas := e.collectRunSnapshotDeltasLocked()
	e.mu.Unlock()
	e.emitRunSnapshotDeltas(deltas)
}
