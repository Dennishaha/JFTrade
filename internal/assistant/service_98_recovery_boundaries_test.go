package assistant

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

// TestCoverage98ServiceSkillRecoveryContracts verifies that the façade keeps
// installation and audit failures observable instead of returning a partial
// skill-management result.
func TestCoverage98ServiceSkillRecoveryContracts(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()

	malformedDir := filepath.Join(runtime.Store().SkillsPath(), "coverage98-malformed-skill")
	if err := os.MkdirAll(malformedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll malformed skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(malformedDir, "SKILL.md"), []byte("---\nname: [\n---\nBroken."), 0o644); err != nil {
		t.Fatalf("WriteFile malformed skill: %v", err)
	}
	if _, err := service.ListSkills(ctx); err == nil {
		t.Fatal("ListSkills accepted a malformed installed skill")
	}
	if err := os.RemoveAll(malformedDir); err != nil {
		t.Fatalf("RemoveAll malformed skill: %v", err)
	}

	previousTransport := http.DefaultTransport
	http.DefaultTransport = coverage98SkillDocumentTransport{document: "---\nname: coverage98-service-skill\ndescription: Service installation recovery test\n---\nUse the checked business contract."}
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	skill, err := service.InstallSkill(ctx, "http://8.8.8.8/coverage98-service-skill.md")
	if err != nil {
		t.Fatalf("InstallSkill controlled public URL: %v", err)
	}
	if skill.ID != "coverage98-service-skill" {
		t.Fatalf("installed skill = %#v", skill)
	}

	installed, err := service.GetAudit(ctx, AuditQuery{Kind: "skill.installed", SubjectID: skill.ID})
	if err != nil {
		t.Fatalf("GetAudit installed skill: %v", err)
	}
	if len(installed) != 1 || installed[0].SubjectID != skill.ID {
		t.Fatalf("installed skill audit = %#v", installed)
	}
	if err := service.DeleteSkill(ctx, "coverage98-missing-skill"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteSkill missing = %v, want os.ErrNotExist", err)
	}
}

func TestCoverage98ServiceAuditAndOptimizationStateRecovery(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t, WithOptimizationRuns(newStubOptimizationRuns(map[string]OptimizationRun{
		"coverage98-failed":    {Status: "failed", Result: "provider failed"},
		"coverage98-completed": {Status: "completed", Result: map[string]any{"sharpe": 1.2}},
		"coverage98-cancelled": {Status: "cancelled"},
	})))
	ctx := t.Context()

	runtime.RecordAudit(ctx, "coverage98.subject.filter", "wanted-subject", "wanted", nil)
	runtime.RecordAudit(ctx, "coverage98.subject.filter", "other-subject", "must be excluded", nil)
	filtered, err := service.GetAudit(ctx, AuditQuery{Kind: "coverage98.subject.filter", SubjectID: "wanted-subject"})
	if err != nil {
		t.Fatalf("GetAudit subject filter: %v", err)
	}
	if len(filtered) != 1 || filtered[0].SubjectID != "wanted-subject" {
		t.Fatalf("GetAudit subject filter = %#v", filtered)
	}

	if _, err := service.GetOptimizationTask(ctx, "coverage98-missing-task"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("GetOptimizationTask missing = %v, want not found", err)
	}
	if _, err := service.CancelOptimizationTask(ctx, "coverage98-missing-task"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("CancelOptimizationTask missing = %v, want not found", err)
	}

	for _, tc := range []struct {
		name  string
		runID string
		want  string
	}{
		{name: "failed run determines task state", runID: "coverage98-failed", want: "failed"},
		{name: "completed run determines task state", runID: "coverage98-completed", want: "completed"},
		{name: "cancelled run determines task state", runID: "coverage98-cancelled", want: "cancelled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := service.optimizationTaskResponse(ctx, jfadk.OptimizationTask{
				ID: "coverage98-state-" + tc.want, Status: "queued", Objective: "preserve terminal state",
				Runs: []jfadk.OptimizationRunRef{{DefinitionID: "definition-" + tc.want, RunID: tc.runID}},
			})
			if response["status"] != tc.want {
				t.Fatalf("optimization response = %#v, want status %q", response, tc.want)
			}
		})
	}

	// A response is still useful to the caller when best-effort state caching
	// cannot write because the request has already been cancelled.
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	response := service.optimizationTaskResponse(cancelledCtx, jfadk.OptimizationTask{
		ID: "coverage98-cancelled-save", Status: "queued", Objective: "fail closed",
		Runs: []jfadk.OptimizationRunRef{{DefinitionID: "definition-failed", RunID: "coverage98-failed"}},
	})
	if response["status"] != "failed" {
		t.Fatalf("cancelled persistence response = %#v, want failed state", response)
	}
}

func TestCoverage98CancelOptimizationTaskPropagatesPersistenceFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	runs := &coverage98CancellingOptimizationRuns{cancel: cancel}
	runtime, service, _ := newAssistantServiceHarness(t, WithOptimizationRuns(runs))

	task, err := runtime.Store().SaveOptimizationTask(t.Context(), jfadk.OptimizationTask{
		ID: "coverage98-cancel-persistence", Status: "queued", Objective: "surface persistence errors",
		Runs: []jfadk.OptimizationRunRef{{DefinitionID: "definition", RunID: "coverage98-active-run"}},
	})
	if err != nil {
		t.Fatalf("SaveOptimizationTask: %v", err)
	}
	if _, err := service.CancelOptimizationTask(ctx, task.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("CancelOptimizationTask persistence failure = %v, want context.Canceled", err)
	}
}

type coverage98SkillDocumentTransport struct {
	document string
}

func (t coverage98SkillDocumentTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(t.document)),
		Request:    request,
	}, nil
}

type coverage98CancellingOptimizationRuns struct {
	cancel context.CancelFunc
}

func (r *coverage98CancellingOptimizationRuns) Get(string) (OptimizationRun, bool) {
	return OptimizationRun{}, false
}

func (r *coverage98CancellingOptimizationRuns) Cancel(string) {
	r.cancel()
}
