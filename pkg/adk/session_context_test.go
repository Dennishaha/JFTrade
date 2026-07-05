package adk

import (
	"context"
	"fmt"
	"strings"
	"testing"

	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool/toolconfirmation"
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
	for index := range 10 {
		role := genai.Role(genai.RoleUser)
		if index%2 == 1 {
			role = genai.Role(genai.RoleModel)
		}
		event := adksession.NewEvent(context.Background(), fmt.Sprintf("inv-%d", index))
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

func TestSessionContextUsesSessionProviderOverrideWindow(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID:                  "context-base-provider",
		DisplayName:         "Context Base",
		BaseURL:             "https://base.example.test",
		Model:               "base-model",
		ContextWindowTokens: 1000,
		APIKey:              "sk-base",
		Enabled:             true,
	})
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID:                  "context-override-provider",
		DisplayName:         "Context Override",
		BaseURL:             "https://override.example.test",
		Model:               "override-model",
		ContextWindowTokens: 200000,
		APIKey:              "sk-override",
		Enabled:             true,
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:             "context-provider-override-agent",
		Name:           "Context Provider Override",
		ProviderID:     "context-base-provider",
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Context Provider Override")
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}
	event := adksession.NewEvent(context.Background(), "context-provider-override-user")
	event.Content = genai.NewContentFromText("hello with override provider", genai.RoleUser)
	if err := runtime.rawSessionService.AppendEvent(ctx, created.Session, event); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	providerOverride := "context-override-provider"
	modelOverride := "override-model"
	if _, err := runtime.Store().SaveSessionComposerState(ctx, session.ID, SessionComposerStatePatch{
		ProviderIDOverride: &providerOverride,
		ModelOverride:      &modelOverride,
	}); err != nil {
		t.Fatalf("SaveSessionComposerState: %v", err)
	}

	snapshot, err := runtime.SessionContext(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionContext: %v", err)
	}
	if snapshot.ContextWindowTokens != 200000 {
		t.Fatalf("context window = %d, want override provider window 200000", snapshot.ContextWindowTokens)
	}
	if snapshot.UsageRatio <= 0 {
		t.Fatalf("usage ratio = %f, want positive ratio from override provider window", snapshot.UsageRatio)
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
	for index := range 6 {
		role := genai.Role(genai.RoleUser)
		if index%2 == 1 {
			role = genai.Role(genai.RoleModel)
		}
		event := adksession.NewEvent(context.Background(), fmt.Sprintf("notice-%d", index))
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
	for index := range 80 {
		role := genai.Role(genai.RoleUser)
		if index%2 == 1 {
			role = genai.Role(genai.RoleModel)
		}
		event := adksession.NewEvent(context.Background(), fmt.Sprintf("auto-notice-%d", index))
		event.Content = genai.NewContentFromText(strings.Repeat(fmt.Sprintf("message %d ", index), 50), role)
		if err := runtime.rawSessionService.AppendEvent(ctx, created.Session, event); err != nil {
			t.Fatalf("AppendEvent(%d): %v", index, err)
		}
	}

	before, err := runtime.contextManager.ProjectedSnapshot(ctx, session, agent, strings.Repeat("pending input ", 200))
	if err != nil {
		t.Fatalf("ProjectedSnapshot before: %v", err)
	}
	var deltas []ChatDelta
	if err := runtime.maybeAutoCompactSession(ctx, session, agent, strings.Repeat("pending input ", 200), func(delta ChatDelta) error {
		deltas = append(deltas, delta)
		return nil
	}); err != nil {
		t.Fatalf("maybeAutoCompactSession: %v", err)
	}
	var notices []TimelineEntry
	var compacted *SessionContextSnapshot
	for _, delta := range deltas {
		if delta.Timeline != nil && delta.Timeline.Kind == TimelineKindContextNotice {
			notices = append(notices, *delta.Timeline)
		}
		if delta.Context != nil {
			compacted = delta.Context
		}
	}
	if len(notices) != 2 {
		t.Fatalf("context notice deltas = %+v, want streaming and final", notices)
	}
	if notices[0].Status != TimelineStatusStreaming || notices[1].Status != TimelineStatusFinal || notices[0].ID != notices[1].ID {
		t.Fatalf("context notices = %+v, want same notice streaming -> final", notices)
	}
	if compacted == nil {
		t.Fatalf("deltas = %+v, want context snapshot after compaction", deltas)
	}
	if compacted.CurrentInputTokens >= before.ProjectedNextTurnTokens {
		t.Fatalf("context tokens after = %d, want less than projected before %d", compacted.CurrentInputTokens, before.ProjectedNextTurnTokens)
	}
	if compacted.ContextRevisionID == "" || compacted.ContextRevisionID == before.ContextRevisionID {
		t.Fatalf("context revision after = %q, before %q", compacted.ContextRevisionID, before.ContextRevisionID)
	}
	if compacted.PreviousContextRevisionID != before.ContextRevisionID {
		t.Fatalf("previous revision = %q, want %q", compacted.PreviousContextRevisionID, before.ContextRevisionID)
	}
	if !compacted.AutoCompacted || compacted.ActiveHandoffCount == 0 || compacted.CompactedEventCount == 0 {
		t.Fatalf("compacted snapshot = %+v, want auto compaction handoff state", compacted)
	}
	segments, err := runtime.Store().HandoffSegmentsForRevision(ctx, session.ID, compacted.ContextRevisionID, true)
	if err != nil {
		t.Fatalf("HandoffSegmentsForRevision: %v", err)
	}
	if len(segments) == 0 {
		t.Fatal("auto compaction created no handoff segment")
	}
	savedNotices, err := runtime.Store().SessionNotices(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionNotices: %v", err)
	}
	if len(savedNotices) != 1 || savedNotices[0].Status != TimelineStatusFinal || savedNotices[0].Text != contextCompactionDoneText {
		t.Fatalf("saved notices = %+v, want one final compaction notice", savedNotices)
	}
}

func TestMaybeAutoCompactSessionSkipsWhenSessionCompactionAlreadyRunning(t *testing.T) {
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
		ID:               "context-auto-gate-agent",
		Name:             "Context Auto Gate Agent",
		Instruction:      "Test agent",
		RecentUserWindow: 1,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Context Auto Gate Session")
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName: googleADKAppName(agent.ID), UserID: googleADKUserID, SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}
	appendLargeContextEvents(t, runtime.rawSessionService, created.Session, 0, 80)

	release, acquired := runtime.beginSessionCompaction(session.ID)
	if !acquired {
		t.Fatal("beginSessionCompaction acquired = false, want true")
	}
	defer release()
	var deltas []ChatDelta
	if err := runtime.maybeAutoCompactSessionDuringWorkflow(ctx, session, agent, strings.Repeat("pending input ", 200), func(delta ChatDelta) error {
		deltas = append(deltas, delta)
		return nil
	}); err != nil {
		t.Fatalf("maybeAutoCompactSessionDuringWorkflow: %v", err)
	}
	if len(deltas) != 0 {
		t.Fatalf("deltas = %+v, want no duplicate compaction notice while gate is held", deltas)
	}
	timeline, ok, err := runtime.Store().SessionTimeline(ctx, session.ID)
	if err != nil || !ok {
		t.Fatalf("SessionTimeline ok=%v err=%v", ok, err)
	}
	for _, entry := range timeline {
		if entry.Kind == TimelineKindContextNotice {
			t.Fatalf("timeline = %+v, want no duplicate context notice while gate is held", timeline)
		}
	}
}

func TestSessionServiceAutoCompactionUsesSessionGate(t *testing.T) {
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
		ID:               "context-service-gate-agent",
		Name:             "Context Service Gate Agent",
		Instruction:      "Test agent",
		RecentUserWindow: 1,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Context Service Gate Session")
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName: googleADKAppName(agent.ID), UserID: googleADKUserID, SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}
	appendLargeContextEvents(t, runtime.rawSessionService, created.Session, 0, 80)

	release, acquired := runtime.beginSessionCompaction(session.ID)
	if !acquired {
		t.Fatal("beginSessionCompaction acquired = false, want true")
	}
	request := &adksession.GetRequest{AppName: googleADKAppName(agent.ID), UserID: googleADKUserID, SessionID: session.ID}
	if _, err := runtime.sessionService.Get(ctx, request); err != nil {
		t.Fatalf("Get while gate held: %v", err)
	}
	before, err := runtime.SessionContext(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionContext before release: %v", err)
	}
	if before.AutoCompacted || before.ActiveHandoffCount != 0 {
		t.Fatalf("context compacted while session gate held: %+v", before)
	}

	release()
	if _, err := runtime.sessionService.Get(ctx, request); err != nil {
		t.Fatalf("Get after gate release: %v", err)
	}
	after, err := runtime.SessionContext(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionContext after release: %v", err)
	}
	if !after.AutoCompacted || after.ActiveHandoffCount == 0 {
		t.Fatalf("context was not compacted after session gate release: %+v", after)
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

func TestProtectedTailStartsAtEarliestUnresolvedApprovalEvent(t *testing.T) {
	events := []*adksession.Event{
		newContextTextEvent("ctx-protect-0", "old user", genai.RoleUser),
		newContextApprovalEvent("ctx-protect-1"),
		newContextTextEvent("ctx-protect-2", "middle", genai.RoleModel),
		newContextTextEvent("ctx-protect-3", "middle user", genai.RoleUser),
		newContextApprovalEvent("ctx-protect-4"),
		newContextTextEvent("ctx-protect-5", "tail", genai.RoleModel),
	}
	if got := protectedTailStart(events); got != 1 {
		t.Fatalf("protectedTailStart = %d, want earliest unresolved approval index 1", got)
	}
}

func TestProtectedTailIncludesOriginalFunctionCallForPendingApproval(t *testing.T) {
	events := []*adksession.Event{
		newContextTextEvent("ctx-original-0", "old user", genai.RoleUser),
		newContextFunctionCallEvent("ctx-original-call", "call-original"),
		newContextFunctionResponseEvent("ctx-original-wait", "call-original", "strategy.research_backtest"),
		newContextApprovalEventForOriginal("ctx-original-approval", "call-original"),
		newContextTextEvent("ctx-original-tail", "tail", genai.RoleModel),
	}
	if got := protectedTailStart(events); got != 1 {
		t.Fatalf("protectedTailStart = %d, want original function call index 1", got)
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
	if err := ValidateSQLiteSessionService(service); err != nil {
		t.Fatalf("ValidateSQLiteSessionService: %v", err)
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

	first := adksession.NewEvent(context.Background(), "inv-first")
	first.Author = "agent"
	first.Content = genai.NewContentFromText("first", genai.RoleModel)
	if err := service.AppendEvent(ctx, fresh.Session, first); err != nil {
		t.Fatalf("AppendEvent(first): %v", err)
	}
	second := adksession.NewEvent(context.Background(), "inv-second")
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

func TestCompactedSessionViewTracksEventsAppendedDuringInvocation(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "context-live-view-agent", Name: "Context Live View", Instruction: "Test agent",
		RecentUserWindow: 1, PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Live projected context")
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName: googleADKAppName(agent.ID), UserID: googleADKUserID, SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}
	appendContextEvents(t, runtime.rawSessionService, created.Session, 0, 6)
	if _, err := runtime.contextManager.Compact(ctx, session, agent, SessionCompactRequest{
		Mode: "aggressive", Trigger: "manual", Reason: "test live projected view",
	}); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	response, err := runtime.sessionService.Get(ctx, &adksession.GetRequest{
		AppName: googleADKAppName(agent.ID), UserID: googleADKUserID, SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Get wrapped session: %v", err)
	}
	before := response.Session.Events().Len()
	call := adksession.NewEvent(context.Background(), "inv-live")
	call.Content = genai.NewContentFromParts([]*genai.Part{{FunctionCall: &genai.FunctionCall{
		ID: "call-live", Name: "test.tool", Args: map[string]any{"value": 1},
	}}}, genai.RoleModel)
	if err := runtime.sessionService.AppendEvent(ctx, response.Session, call); err != nil {
		t.Fatalf("Append call: %v", err)
	}
	result := adksession.NewEvent(context.Background(), "inv-live")
	result.Content = genai.NewContentFromParts([]*genai.Part{{FunctionResponse: &genai.FunctionResponse{
		ID: "call-live", Name: "test.tool", Response: map[string]any{"ok": true},
	}}}, genai.RoleUser)
	if err := runtime.sessionService.AppendEvent(ctx, response.Session, result); err != nil {
		t.Fatalf("Append response: %v", err)
	}

	events := eventSlice(response.Session.Events())
	if got := len(events); got != before+2 {
		t.Fatalf("projected event count = %d, want %d", got, before+2)
	}
	if len(events[len(events)-2].Content.Parts) == 0 || events[len(events)-2].Content.Parts[0].FunctionCall == nil {
		t.Fatalf("projected view lost appended function call")
	}
	if len(events[len(events)-1].Content.Parts) == 0 || events[len(events)-1].Content.Parts[0].FunctionResponse == nil {
		t.Fatalf("projected view lost appended function response")
	}
}

func TestHasActiveRunDoesNotTreatPendingApprovalAsExecuting(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "context-active-run-agent", Name: "Context Active Run", Instruction: "Test agent",
		RecentUserWindow: 1, PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Active run guard")
	pending := mustSaveRun(t, runtime, Run{
		ID: "run-context-pending", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusPending, CreatedAt: nowString(), UpdatedAt: nowString(),
	})
	active, err := runtime.contextManager.HasActiveRun(ctx, session.ID)
	if err != nil {
		t.Fatalf("HasActiveRun pending: %v", err)
	}
	if active {
		t.Fatalf("pending approval run must not block compaction")
	}
	pending.Status = RunStatusRunning
	if err := runtime.Store().SaveRun(ctx, pending); err != nil {
		t.Fatalf("Save running run: %v", err)
	}
	active, err = runtime.contextManager.HasActiveRun(ctx, session.ID)
	if err != nil {
		t.Fatalf("HasActiveRun running: %v", err)
	}
	if !active {
		t.Fatalf("running run must block manual compaction")
	}
}

func TestCompactedSessionPreservesOriginalCallForPendingApproval(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "context-pending-pair-agent", Name: "Context Pending Pair", Instruction: "Test agent",
		RecentUserWindow: 1, PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Pending approval pair")
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName: googleADKAppName(agent.ID), UserID: googleADKUserID, SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}
	appendContextEvents(t, runtime.rawSessionService, created.Session, 0, 8)
	events := []*adksession.Event{
		newContextFunctionCallEvent("ctx-pair-call", "call-pair-original"),
		newContextFunctionResponseEvent("ctx-pair-wait", "call-pair-original", "strategy.research_backtest"),
		newContextApprovalEventForOriginal("ctx-pair-approval", "call-pair-original"),
	}
	for _, event := range events {
		if err := appendADKEventWithStaleRetry(ctx, runtime.contextManager.appendLocks, runtime.rawSessionService, created.Session, event); err != nil {
			t.Fatalf("Append pending pair event: %v", err)
		}
	}
	if _, err := runtime.contextManager.Compact(ctx, session, agent, SessionCompactRequest{
		Mode: "aggressive", Trigger: "manual", Reason: "test pending approval pair",
	}); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	response, err := runtime.sessionService.Get(ctx, &adksession.GetRequest{
		AppName: googleADKAppName(agent.ID), UserID: googleADKUserID, SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Get projected session: %v", err)
	}
	seenOriginal := false
	seenConfirmation := false
	for event := range response.Session.Events().All() {
		if event == nil || event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part == nil || part.FunctionCall == nil {
				continue
			}
			if part.FunctionCall.ID == "call-pair-original" {
				seenOriginal = true
			}
			if part.FunctionCall.Name == toolconfirmation.FunctionCallName {
				seenConfirmation = true
			}
		}
	}
	if !seenOriginal || !seenConfirmation {
		t.Fatalf("projected pending approval pair original=%v confirmation=%v", seenOriginal, seenConfirmation)
	}
}
