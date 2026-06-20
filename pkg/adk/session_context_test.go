package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	adksession "google.golang.org/adk/session"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
)

func TestSessionContextCompactionShrinksSessionView(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID:               "context-agent",
		Name:             "Context Agent",
		Instruction:      "Test agent",
		RecentUserWindow: 2,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := runtime.Store().CreateSession(ctx, agent.ID, "Context Session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}
	for index := 0; index < 10; index++ {
		role := genai.Role(genai.RoleUser)
		if index%2 == 1 {
			role = genai.Role(genai.RoleModel)
		}
		event := adksession.NewEvent(fmt.Sprintf("inv-%d", index))
		event.Content = genai.NewContentFromText(fmt.Sprintf("message %d", index), role)
		if err := runtime.rawSessionService.AppendEvent(ctx, created.Session, event); err != nil {
			t.Fatalf("AppendEvent(%d): %v", index, err)
		}
	}

	snapshotBefore, err := runtime.contextManager.Snapshot(ctx, session, agent)
	if err != nil {
		t.Fatalf("Snapshot before: %v", err)
	}
	if snapshotBefore.RawEventCount != 10 {
		t.Fatalf("RawEventCount before = %d, want 10", snapshotBefore.RawEventCount)
	}

	snapshotAfter, err := runtime.contextManager.Compact(ctx, session, agent, SessionCompactRequest{
		Mode:    "normal",
		Trigger: "manual",
		Reason:  "test compaction",
	})
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if snapshotAfter.CompactedEventCount == 0 {
		t.Fatalf("CompactedEventCount = 0, want > 0")
	}
	if snapshotAfter.ProtectedRecentCount >= snapshotAfter.RawEventCount {
		t.Fatalf("ProtectedRecentCount = %d, want less than raw count %d", snapshotAfter.ProtectedRecentCount, snapshotAfter.RawEventCount)
	}
	if snapshotAfter.SummaryPreview == "" {
		t.Fatalf("SummaryPreview is empty")
	}
	rawAfterCompact, err := runtime.rawSessionService.Get(ctx, &adksession.GetRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Get raw session after compact: %v", err)
	}
	stateSummary, err := rawAfterCompact.Session.State().Get(adkSessionHandoffSummaryKey)
	if err != nil {
		t.Fatalf("ADK handoff state missing: %v", err)
	}
	if strings.TrimSpace(fmt.Sprint(stateSummary)) == "" {
		t.Fatalf("ADK handoff state is empty")
	}
	suffix, err := runtime.contextManager.InstructionSuffix(ctx, session.ID)
	if err != nil {
		t.Fatalf("InstructionSuffix: %v", err)
	}
	if !strings.Contains(suffix, strings.TrimSpace(fmt.Sprint(stateSummary))) {
		t.Fatalf("InstructionSuffix does not include ADK handoff state")
	}

	response, err := runtime.sessionService.Get(ctx, &adksession.GetRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("wrapped Get: %v", err)
	}
	if got, want := response.Session.Events().Len(), snapshotAfter.RetainedRecentUserCount; got != want {
		t.Fatalf("wrapped view len = %d, want %d", got, want)
	}
	for event := range response.Session.Events().All() {
		if !isUserEvent(event) {
			t.Fatalf("wrapped view contains non-user event after compaction")
		}
	}
}

func TestSessionContextCompactionCreatesCurrentRevision(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:               "context-revision-agent",
		Name:             "Context Revision Agent",
		Instruction:      "Test agent",
		RecentUserWindow: 1,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Context Revision")
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}
	appendContextEvents(t, runtime.rawSessionService, created.Session, 0, 12)

	before, err := runtime.contextManager.Snapshot(ctx, session, agent)
	if err != nil {
		t.Fatalf("Snapshot before: %v", err)
	}
	first, err := runtime.contextManager.Compact(ctx, session, agent, SessionCompactRequest{
		Mode:    "normal",
		Trigger: "manual",
		Reason:  "first",
	})
	if err != nil {
		t.Fatalf("first Compact: %v", err)
	}
	if first.SessionID != session.ID {
		t.Fatalf("SessionID = %q, want %q", first.SessionID, session.ID)
	}
	if first.ContextRevisionID == "" || first.ContextRevisionID == before.ContextRevisionID {
		t.Fatalf("first revision = %q, before %q", first.ContextRevisionID, before.ContextRevisionID)
	}
	if first.PreviousContextRevisionID != before.ContextRevisionID {
		t.Fatalf("first previous revision = %q, want %q", first.PreviousContextRevisionID, before.ContextRevisionID)
	}
	firstSegments, err := runtime.Store().HandoffSegmentsForRevision(ctx, session.ID, first.ContextRevisionID, true)
	if err != nil {
		t.Fatalf("first HandoffSegmentsForRevision: %v", err)
	}
	if len(firstSegments) != 1 {
		t.Fatalf("first current segments = %d, want 1", len(firstSegments))
	}

	latest, err := runtime.rawSessionService.Get(ctx, &adksession.GetRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Get latest raw session: %v", err)
	}
	appendContextEvents(t, runtime.rawSessionService, latest.Session, 12, 4)
	second, err := runtime.contextManager.Compact(ctx, session, agent, SessionCompactRequest{
		Mode:    "normal",
		Trigger: "manual",
		Reason:  "second",
	})
	if err != nil {
		t.Fatalf("second Compact: %v", err)
	}
	if second.ContextRevisionID == first.ContextRevisionID {
		t.Fatalf("second revision = first revision %q", second.ContextRevisionID)
	}
	if second.PreviousContextRevisionID != first.ContextRevisionID {
		t.Fatalf("second previous revision = %q, want %q", second.PreviousContextRevisionID, first.ContextRevisionID)
	}
	secondSegments, err := runtime.Store().HandoffSegmentsForRevision(ctx, session.ID, second.ContextRevisionID, true)
	if err != nil {
		t.Fatalf("second HandoffSegmentsForRevision: %v", err)
	}
	if len(secondSegments) != 1 {
		t.Fatalf("second current segments = %d, want 1", len(secondSegments))
	}
	if second.ActiveHandoffCount != len(secondSegments) {
		t.Fatalf("ActiveHandoffCount = %d, want %d", second.ActiveHandoffCount, len(secondSegments))
	}
	latestRevisionTokens := estimateHandoffTokens(secondSegments)
	if second.Breakdown.HandoffTokens != latestRevisionTokens {
		t.Fatalf("handoff tokens = %d, want latest revision tokens %d", second.Breakdown.HandoffTokens, latestRevisionTokens)
	}
	allActiveSegments, err := runtime.Store().HandoffSegments(ctx, session.ID, true)
	if err != nil {
		t.Fatalf("all active HandoffSegments: %v", err)
	}
	if len(allActiveSegments) <= len(secondSegments) {
		t.Fatalf("all active segments = %d, want more than current revision segments %d", len(allActiveSegments), len(secondSegments))
	}
	allActiveTokens := estimateHandoffTokens(allActiveSegments)
	if second.Breakdown.HandoffTokens == allActiveTokens {
		t.Fatalf("handoff tokens use all active revisions: activeTokens=%d snapshot=%d", allActiveTokens, second.Breakdown.HandoffTokens)
	}
	if second.RawEventCount <= second.CompactedEventCount {
		t.Fatalf("raw diagnostics collapsed into compacted view: raw=%d compacted=%d", second.RawEventCount, second.CompactedEventCount)
	}
}

func TestCompactSessionContextWritesContextNotice(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:               "context-notice-agent",
		Name:             "Context Notice Agent",
		Instruction:      "Test agent",
		RecentUserWindow: 1,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Context Notice Session")
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}
	for index := 0; index < 6; index++ {
		role := genai.Role(genai.RoleUser)
		if index%2 == 1 {
			role = genai.Role(genai.RoleModel)
		}
		event := adksession.NewEvent(fmt.Sprintf("notice-%d", index))
		event.Content = genai.NewContentFromText(fmt.Sprintf("message %d", index), role)
		if err := runtime.rawSessionService.AppendEvent(ctx, created.Session, event); err != nil {
			t.Fatalf("AppendEvent(%d): %v", index, err)
		}
	}

	if _, err := runtime.CompactSessionContext(ctx, session.ID, "normal", "manual", "test notice"); err != nil {
		t.Fatalf("CompactSessionContext: %v", err)
	}
	timeline, ok, err := runtime.Store().SessionTimeline(ctx, session.ID)
	if err != nil || !ok {
		t.Fatalf("SessionTimeline ok=%v err=%v", ok, err)
	}
	found := false
	for _, entry := range timeline {
		if entry.Kind != TimelineKindContextNotice {
			continue
		}
		found = true
		if entry.Status != TimelineStatusFinal || entry.Text != contextCompactionDoneText {
			t.Fatalf("context notice = %+v, want final done notice", entry)
		}
	}
	if !found {
		t.Fatalf("timeline = %+v, want context notice", timeline)
	}
}

func TestMaybeAutoCompactSessionEmitsContextNoticeDeltas(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	provider, ok, err := runtime.Store().Provider(ctx, testProviderID)
	if err != nil || !ok {
		t.Fatalf("Provider: ok=%v err=%v", ok, err)
	}
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID:                  testProviderID,
		DisplayName:         provider.DisplayName,
		BaseURL:             provider.BaseURL,
		Model:               provider.Model,
		APIKey:              "sk-test",
		ContextWindowTokens: 80,
		RequestTimeoutMs:    5000,
		Enabled:             true,
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:               "context-auto-notice-agent",
		Name:             "Context Auto Notice Agent",
		Instruction:      "Test agent",
		RecentUserWindow: 1,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Context Auto Notice Session")
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}
	for index := 0; index < 80; index++ {
		role := genai.Role(genai.RoleUser)
		if index%2 == 1 {
			role = genai.Role(genai.RoleModel)
		}
		event := adksession.NewEvent(fmt.Sprintf("auto-notice-%d", index))
		event.Content = genai.NewContentFromText(strings.Repeat(fmt.Sprintf("message %d ", index), 50), role)
		if err := runtime.rawSessionService.AppendEvent(ctx, created.Session, event); err != nil {
			t.Fatalf("AppendEvent(%d): %v", index, err)
		}
	}

	var deltas []ChatDelta
	if err := runtime.maybeAutoCompactSession(ctx, session, agent, strings.Repeat("pending input ", 200), func(delta ChatDelta) error {
		deltas = append(deltas, delta)
		return nil
	}); err != nil {
		t.Fatalf("maybeAutoCompactSession: %v", err)
	}
	var notices []TimelineEntry
	hasContext := false
	for _, delta := range deltas {
		if delta.Timeline != nil && delta.Timeline.Kind == TimelineKindContextNotice {
			notices = append(notices, *delta.Timeline)
		}
		if delta.Context != nil {
			hasContext = true
		}
	}
	if len(notices) != 2 {
		t.Fatalf("context notice deltas = %+v, want streaming and final", notices)
	}
	if notices[0].Status != TimelineStatusStreaming || notices[1].Status != TimelineStatusFinal || notices[0].ID != notices[1].ID {
		t.Fatalf("context notices = %+v, want same notice streaming -> final", notices)
	}
	if !hasContext {
		t.Fatalf("deltas = %+v, want context snapshot after compaction", deltas)
	}
}

func TestMaybeAutoCompactSessionDuringWorkflowAllowsActiveParent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	provider, ok, err := runtime.Store().Provider(ctx, testProviderID)
	if err != nil || !ok {
		t.Fatalf("Provider: ok=%v err=%v", ok, err)
	}
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID:                  testProviderID,
		DisplayName:         provider.DisplayName,
		BaseURL:             provider.BaseURL,
		Model:               provider.Model,
		APIKey:              "sk-test",
		ContextWindowTokens: 80,
		RequestTimeoutMs:    5000,
		Enabled:             true,
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:               "context-workflow-auto-agent",
		Name:             "Context Workflow Auto Agent",
		Instruction:      "Test agent",
		RecentUserWindow: 1,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Context Workflow Auto Session")
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}
	appendLargeContextEvents(t, runtime.rawSessionService, created.Session, 0, 80)
	mustSaveRun(t, runtime, Run{
		ID:        "run-active-workflow-parent",
		SessionID: session.ID,
		AgentID:   agent.ID,
		Status:    RunStatusRunning,
		WorkMode:  WorkModeLoop,
		CreatedAt: nowString(),
		UpdatedAt: nowString(),
	})

	var skipped []ChatDelta
	if err := runtime.maybeAutoCompactSession(ctx, session, agent, strings.Repeat("pending input ", 200), func(delta ChatDelta) error {
		skipped = append(skipped, delta)
		return nil
	}); err != nil {
		t.Fatalf("maybeAutoCompactSession: %v", err)
	}
	if len(skipped) != 0 {
		t.Fatalf("regular auto compaction deltas = %+v, want skipped while active", skipped)
	}

	var deltas []ChatDelta
	if err := runtime.maybeAutoCompactSessionDuringWorkflow(ctx, session, agent, strings.Repeat("pending input ", 200), func(delta ChatDelta) error {
		deltas = append(deltas, delta)
		return nil
	}); err != nil {
		t.Fatalf("maybeAutoCompactSessionDuringWorkflow: %v", err)
	}
	hasContext := false
	for _, delta := range deltas {
		if delta.Context != nil {
			hasContext = true
			break
		}
	}
	if !hasContext {
		t.Fatalf("workflow auto compaction deltas = %+v, want context snapshot", deltas)
	}
	snapshot, err := runtime.SessionContext(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionContext: %v", err)
	}
	if !snapshot.AutoCompacted || snapshot.ActiveHandoffCount == 0 {
		t.Fatalf("snapshot = %+v, want auto compacted handoff", snapshot)
	}
}

func TestSessionContextViewDoesNotAutoCompact(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	provider, ok, err := runtime.Store().Provider(ctx, testProviderID)
	if err != nil || !ok {
		t.Fatalf("Provider: ok=%v err=%v", ok, err)
	}
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID:                  testProviderID,
		DisplayName:         provider.DisplayName,
		BaseURL:             provider.BaseURL,
		Model:               provider.Model,
		APIKey:              "sk-test",
		ContextWindowTokens: 80,
		RequestTimeoutMs:    5000,
		Enabled:             true,
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:               "context-view-no-write-agent",
		Name:             "Context View No Write Agent",
		Instruction:      "Test agent",
		RecentUserWindow: 1,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Context View No Write")
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}
	appendLargeContextEvents(t, runtime.rawSessionService, created.Session, 0, 80)

	snapshot, err := runtime.SessionContext(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionContext: %v", err)
	}
	if snapshot.Status != ContextStatusCritical && snapshot.Status != ContextStatusNearLimit {
		t.Fatalf("snapshot status = %q, want near limit or critical", snapshot.Status)
	}
	segments, err := runtime.Store().HandoffSegmentsForRevision(ctx, session.ID, snapshot.ContextRevisionID, true)
	if err != nil {
		t.Fatalf("HandoffSegmentsForRevision: %v", err)
	}
	if len(segments) != 0 || snapshot.ActiveHandoffCount != 0 || snapshot.AutoCompacted {
		t.Fatalf("snapshot=%+v segments=%+v, want context view without auto compaction", snapshot, segments)
	}
}

func TestModelContextReadAutoCompactsBeforeProviderPayload(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	provider, ok, err := runtime.Store().Provider(ctx, testProviderID)
	if err != nil || !ok {
		t.Fatalf("Provider: ok=%v err=%v", ok, err)
	}
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID:                  testProviderID,
		DisplayName:         provider.DisplayName,
		BaseURL:             provider.BaseURL,
		Model:               provider.Model,
		APIKey:              "sk-test",
		ContextWindowTokens: 80,
		RequestTimeoutMs:    5000,
		Enabled:             true,
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:               "context-model-auto-agent",
		Name:             "Context Model Auto Agent",
		Instruction:      "Test agent",
		RecentUserWindow: 1,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Context Model Auto")
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}
	appendLargeContextEvents(t, runtime.rawSessionService, created.Session, 0, 80)
	before, err := runtime.SessionContext(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionContext before: %v", err)
	}
	if before.ActiveHandoffCount != 0 {
		t.Fatalf("before.ActiveHandoffCount = %d, want 0", before.ActiveHandoffCount)
	}

	response, err := runtime.sessionService.Get(ctx, &adksession.GetRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("wrapped session Get: %v", err)
	}
	if response.Session.Events().Len() >= before.RawEventCount {
		t.Fatalf("visible model events = %d, want fewer than raw %d after compaction", response.Session.Events().Len(), before.RawEventCount)
	}
	after, err := runtime.SessionContext(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionContext after: %v", err)
	}
	if !after.AutoCompacted || after.ActiveHandoffCount == 0 || after.CompactedEventCount == 0 {
		t.Fatalf("after snapshot = %+v, want model-context auto compaction", after)
	}
	segments, err := runtime.Store().HandoffSegmentsForRevision(ctx, session.ID, after.ContextRevisionID, true)
	if err != nil {
		t.Fatalf("HandoffSegmentsForRevision: %v", err)
	}
	if len(segments) == 0 || maxActiveSegmentEnd(segments) == 0 {
		t.Fatalf("segments = %+v, want active auto handoff", segments)
	}
}

func TestProtectedTailStartsAtLatestUnresolvedApprovalEvent(t *testing.T) {
	events := []*adksession.Event{
		newContextTextEvent("ctx-protect-0", "old user", genai.RoleUser),
		newContextApprovalEvent("ctx-protect-1"),
		newContextTextEvent("ctx-protect-2", "middle", genai.RoleModel),
		newContextTextEvent("ctx-protect-3", "middle user", genai.RoleUser),
		newContextApprovalEvent("ctx-protect-4"),
		newContextTextEvent("ctx-protect-5", "tail", genai.RoleModel),
	}
	if got := protectedTailStart(events); got != 4 {
		t.Fatalf("protectedTailStart = %d, want latest approval index 4", got)
	}
}

func TestProtectedTailIgnoresResolvedApprovalEvent(t *testing.T) {
	events := []*adksession.Event{
		newContextTextEvent("ctx-resolved-0", "old user", genai.RoleUser),
		newContextApprovalEvent("ctx-resolved-1"),
		newContextTextEvent("ctx-resolved-2", "middle", genai.RoleModel),
		newContextApprovalResponseEvent("ctx-resolved-1"),
		newContextTextEvent("ctx-resolved-4", "tail", genai.RoleModel),
	}
	if got := protectedTailStart(events); got != len(events) {
		t.Fatalf("protectedTailStart = %d, want no protected tail", got)
	}
}

func TestProtectedTailKeepsOnlyUnresolvedApprovalWhenOlderApprovalResolved(t *testing.T) {
	events := []*adksession.Event{
		newContextTextEvent("ctx-mixed-0", "old user", genai.RoleUser),
		newContextApprovalEvent("ctx-mixed-1"),
		newContextApprovalResponseEvent("ctx-mixed-1"),
		newContextTextEvent("ctx-mixed-3", "middle", genai.RoleModel),
		newContextApprovalEvent("ctx-mixed-4"),
		newContextTextEvent("ctx-mixed-5", "tail", genai.RoleModel),
	}
	if got := protectedTailStart(events); got != 4 {
		t.Fatalf("protectedTailStart = %d, want unresolved approval index 4", got)
	}
}

func TestSessionContextIgnoresHandoffSegmentsWithoutRevision(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:               "context-no-legacy-agent",
		Name:             "Context No Legacy Agent",
		Instruction:      "Test agent",
		RecentUserWindow: 1,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "No Legacy Context")
	_, err := runtime.Store().db.ExecContext(ctx,
		`INSERT INTO `+tableHandoffSegments+` (id, session_id, active, sequence_no, created_at, updated_at, payload_json) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"old-handoff-without-revision", session.ID, 1, 1, nowString(), nowString(),
		`{"id":"old-handoff-without-revision","sessionId":"`+session.ID+`","sequence":1,"startEventIndex":0,"endEventIndex":1,"summary":"old summary","mode":"manual","estimatedTokens":2,"active":true,"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}`,
	)
	if err != nil {
		t.Fatalf("insert old handoff: %v", err)
	}
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}
	appendContextEvents(t, runtime.rawSessionService, created.Session, 0, 2)
	snapshot, err := runtime.contextManager.Snapshot(ctx, session, agent)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snapshot.ContextRevisionID == "" {
		t.Fatalf("ContextRevisionID is empty")
	}
	if snapshot.SummaryPreview != "" || snapshot.ActiveHandoffCount != 0 {
		t.Fatalf("snapshot used old handoff segment: %+v", snapshot)
	}
}

func TestAppendADKEventWithStaleRetryRefreshesSession(t *testing.T) {
	ctx := context.Background()
	service, err := NewSQLiteSessionService(t.TempDir() + "/adk-session.db")
	if err != nil {
		t.Fatalf("NewSQLiteSessionService: %v", err)
	}
	t.Cleanup(func() { jftradeErr1 := CloseSessionService(service); jftradeCheckTestError(t, jftradeErr1) })
	if err := MigrateSQLiteSessionService(service); err != nil {
		t.Fatalf("MigrateSQLiteSessionService: %v", err)
	}
	created, err := service.Create(ctx, &adksession.CreateRequest{
		AppName: "app", UserID: "user", SessionID: "session-stale-retry",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	stale := created.Session
	fresh, err := service.Get(ctx, &adksession.GetRequest{
		AppName: "app", UserID: "user", SessionID: "session-stale-retry",
	})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	first := adksession.NewEvent("inv-first")
	first.Author = "agent"
	first.Content = genai.NewContentFromText("first", genai.RoleModel)
	if err := service.AppendEvent(ctx, fresh.Session, first); err != nil {
		t.Fatalf("AppendEvent(first): %v", err)
	}
	second := adksession.NewEvent("inv-second")
	second.Author = "agent"
	second.Content = genai.NewContentFromText("second", genai.RoleModel)
	locks := newADKSessionAppendLockMap()
	if err := appendADKEventWithStaleRetry(ctx, locks, service, stale, second); err != nil {
		t.Fatalf("appendADKEventWithStaleRetry: %v", err)
	}
	if locks.len() != 0 {
		t.Fatalf("append lock count = %d, want 0", locks.len())
	}
	latest, err := service.Get(ctx, &adksession.GetRequest{
		AppName: "app", UserID: "user", SessionID: "session-stale-retry",
	})
	if err != nil {
		t.Fatalf("Get latest: %v", err)
	}
	if latest.Session.Events().Len() != 2 {
		t.Fatalf("event count = %d, want 2", latest.Session.Events().Len())
	}
}

func appendContextEvents(t *testing.T, service adksession.Service, session adksession.Session, start int, count int) {
	t.Helper()
	for index := start; index < start+count; index++ {
		role := genai.Role(genai.RoleUser)
		if index%2 == 1 {
			role = genai.Role(genai.RoleModel)
		}
		event := adksession.NewEvent(fmt.Sprintf("ctx-%d", index))
		event.Content = genai.NewContentFromText(fmt.Sprintf("message %d", index), role)
		if err := appendADKEventWithStaleRetry(context.Background(), newADKSessionAppendLockMap(), service, session, event); err != nil {
			t.Fatalf("Append context event %d: %v", index, err)
		}
	}
}

func appendLargeContextEvents(t *testing.T, service adksession.Service, session adksession.Session, start int, count int) {
	t.Helper()
	for index := start; index < start+count; index++ {
		role := genai.Role(genai.RoleUser)
		if index%2 == 1 {
			role = genai.Role(genai.RoleModel)
		}
		event := newContextTextEvent(fmt.Sprintf("large-ctx-%d", index), strings.Repeat(fmt.Sprintf("message %d ", index), 50), role)
		if err := appendADKEventWithStaleRetry(context.Background(), newADKSessionAppendLockMap(), service, session, event); err != nil {
			t.Fatalf("Append large context event %d: %v", index, err)
		}
	}
}

func newContextTextEvent(id string, text string, role genai.Role) *adksession.Event {
	event := adksession.NewEvent(id)
	event.Content = genai.NewContentFromText(text, role)
	return event
}

func newContextApprovalEvent(id string) *adksession.Event {
	event := adksession.NewEvent(id)
	event.Content = genai.NewContentFromParts([]*genai.Part{{
		FunctionCall: &genai.FunctionCall{
			ID:   id + "-call",
			Name: toolconfirmation.FunctionCallName,
			Args: map[string]any{},
		},
	}}, genai.RoleModel)
	return event
}

func newContextApprovalResponseEvent(approvalEventID string) *adksession.Event {
	event := adksession.NewEvent(approvalEventID + "-response")
	event.Content = genai.NewContentFromParts([]*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{
			ID:       approvalEventID + "-call",
			Name:     toolconfirmation.FunctionCallName,
			Response: map[string]any{"approved": true},
		},
	}}, genai.RoleUser)
	return event
}

func TestAppendADKEventWithStaleRetrySerializesConcurrentStaleSession(t *testing.T) {
	ctx := context.Background()
	service, err := NewSQLiteSessionService(t.TempDir() + "/adk-session.db")
	if err != nil {
		t.Fatalf("NewSQLiteSessionService: %v", err)
	}
	t.Cleanup(func() { jftradeErr2 := CloseSessionService(service); jftradeCheckTestError(t, jftradeErr2) })
	if err := MigrateSQLiteSessionService(service); err != nil {
		t.Fatalf("MigrateSQLiteSessionService: %v", err)
	}
	created, err := service.Create(ctx, &adksession.CreateRequest{
		AppName: "app", UserID: "user", SessionID: "session-concurrent-stale-retry",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	const eventCount = 12
	locks := newADKSessionAppendLockMap()
	var wg sync.WaitGroup
	errs := make(chan error, eventCount)
	for index := 0; index < eventCount; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			event := adksession.NewEvent(fmt.Sprintf("inv-concurrent-%02d", index))
			event.Author = "agent"
			event.Content = genai.NewContentFromText(fmt.Sprintf("event-%02d", index), genai.RoleModel)
			if err := appendADKEventWithStaleRetry(ctx, locks, service, created.Session, event); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("appendADKEventWithStaleRetry concurrent error: %v", err)
	}

	latest, err := service.Get(ctx, &adksession.GetRequest{
		AppName: "app", UserID: "user", SessionID: "session-concurrent-stale-retry",
	})
	if err != nil {
		t.Fatalf("Get latest: %v", err)
	}
	if latest.Session.Events().Len() != eventCount {
		t.Fatalf("event count = %d, want %d", latest.Session.Events().Len(), eventCount)
	}
	if locks.len() != 0 {
		t.Fatalf("append lock count = %d, want 0", locks.len())
	}
}

func TestAppendADKEventWithStaleRetryReturnsNonStaleError(t *testing.T) {
	ctx := context.Background()
	base := adksession.InMemoryService()
	created, err := base.Create(ctx, &adksession.CreateRequest{
		AppName: "app", UserID: "user", SessionID: "session-non-stale-error",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	appendErr := errors.New("disk full")
	service := &appendErrorSessionService{Service: base, err: appendErr}
	event := adksession.NewEvent("inv-non-stale")
	event.Author = "agent"
	event.Content = genai.NewContentFromText("non-stale", genai.RoleModel)

	locks := newADKSessionAppendLockMap()
	err = appendADKEventWithStaleRetry(ctx, locks, service, created.Session, event)
	if !errors.Is(err, appendErr) {
		t.Fatalf("append error = %v, want %v", err, appendErr)
	}
	if locks.len() != 0 {
		t.Fatalf("append lock count = %d, want 0", locks.len())
	}
	if service.getCalls != 0 {
		t.Fatalf("Get calls = %d, want 0 for non-stale error", service.getCalls)
	}
}

type appendErrorSessionService struct {
	adksession.Service
	err      error
	getCalls int
}

func (s *appendErrorSessionService) Get(ctx context.Context, req *adksession.GetRequest) (*adksession.GetResponse, error) {
	s.getCalls++
	return s.Service.Get(ctx, req)
}

func (s *appendErrorSessionService) AppendEvent(context.Context, adksession.Session, *adksession.Event) error {
	return s.err
}

func TestSessionContextProjectionTrimsOversizedToolResponses(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	provider, err := runtime.Store().SaveProvider(ctx, ProviderWriteRequest{
		ID:                  "context-provider",
		DisplayName:         "Context Provider",
		BaseURL:             "https://api.openai.com/v1",
		Model:               "gpt-4o-mini",
		ContextWindowTokens: 200000,
		Enabled:             true,
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID:               "context-trim-agent",
		Name:             "Context Trim Agent",
		Instruction:      "Trim oversized tool responses.",
		ProviderID:       provider.ID,
		RecentUserWindow: 2,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := runtime.Store().CreateSession(ctx, agent.ID, "Oversized Session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}

	userEvent := adksession.NewEvent("inv-user")
	userEvent.Content = genai.NewContentFromText("制作一个 tme 的策略", genai.RoleUser)
	if err := runtime.rawSessionService.AppendEvent(ctx, created.Session, userEvent); err != nil {
		t.Fatalf("Append user event: %v", err)
	}

	oversizedPayload := strings.Repeat("x", MaxToolOutputBytes*2)
	toolEvent := adksession.NewEvent("inv-tool")
	toolEvent.Content = genai.NewContentFromParts([]*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{
			ID:   "call-oversized",
			Name: "backtest.runs",
			Response: map[string]any{
				"payload": oversizedPayload,
			},
		},
	}}, genai.RoleModel)
	if err := runtime.rawSessionService.AppendEvent(ctx, created.Session, toolEvent); err != nil {
		t.Fatalf("Append tool event: %v", err)
	}

	snapshot, err := runtime.contextManager.Snapshot(ctx, session, agent)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snapshot.TrimmedToolResponseCount != 1 {
		t.Fatalf("TrimmedToolResponseCount = %d, want 1", snapshot.TrimmedToolResponseCount)
	}
	if snapshot.RawCurrentInputTokens <= snapshot.CurrentInputTokens {
		t.Fatalf("raw current tokens = %d, effective = %d, want raw > effective", snapshot.RawCurrentInputTokens, snapshot.CurrentInputTokens)
	}
	if snapshot.RawBreakdown.OtherVisibleTokens <= snapshot.Breakdown.OtherVisibleTokens {
		t.Fatalf("raw other visible = %d, effective = %d, want raw > effective", snapshot.RawBreakdown.OtherVisibleTokens, snapshot.Breakdown.OtherVisibleTokens)
	}

	response, err := runtime.sessionService.Get(ctx, &adksession.GetRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("wrapped Get: %v", err)
	}
	foundTruncated := false
	for event := range response.Session.Events().All() {
		if event == nil || event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part == nil || part.FunctionResponse == nil {
				continue
			}
			if truncated, ok := part.FunctionResponse.Response["truncated"].(bool); ok && truncated {
				foundTruncated = true
			}
			if payload, ok := part.FunctionResponse.Response["payload"].(string); ok && payload == oversizedPayload {
				t.Fatal("wrapped session still exposes oversized raw payload")
			}
		}
	}
	if !foundTruncated {
		t.Fatal("expected wrapped session to expose truncated tool response preview")
	}
}

func TestSessionContextProjectionKeepsSmallToolResponsesUntouched(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID:               "context-small-agent",
		Name:             "Context Small Agent",
		Instruction:      "Keep small tool responses intact.",
		RecentUserWindow: 2,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := runtime.Store().CreateSession(ctx, agent.ID, "Small Session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}

	toolEvent := adksession.NewEvent("inv-tool-small")
	toolEvent.Content = genai.NewContentFromParts([]*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{
			ID:   "call-small",
			Name: "strategy.definitions",
			Response: map[string]any{
				"definitions": []map[string]any{{"id": "demo", "name": "Demo"}},
			},
		},
	}}, genai.RoleModel)
	if err := runtime.rawSessionService.AppendEvent(ctx, created.Session, toolEvent); err != nil {
		t.Fatalf("Append tool event: %v", err)
	}

	snapshot, err := runtime.contextManager.Snapshot(ctx, session, agent)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snapshot.TrimmedToolResponseCount != 0 {
		t.Fatalf("TrimmedToolResponseCount = %d, want 0", snapshot.TrimmedToolResponseCount)
	}
	if snapshot.RawCurrentInputTokens != snapshot.CurrentInputTokens {
		t.Fatalf("raw current tokens = %d, effective = %d, want equal", snapshot.RawCurrentInputTokens, snapshot.CurrentInputTokens)
	}

	response, err := runtime.sessionService.Get(ctx, &adksession.GetRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("wrapped Get: %v", err)
	}
	for event := range response.Session.Events().All() {
		if event == nil || event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part == nil || part.FunctionResponse == nil {
				continue
			}
			if _, truncated := part.FunctionResponse.Response["truncated"]; truncated {
				t.Fatal("small tool response should not be truncated")
			}
		}
	}
}
