package adk

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestChatRequestIdentityValidationAndFingerprintConflict(t *testing.T) {
	if _, _, err := NormalizeChatRequestIdentity(ChatRequest{Message: "hello"}); err == nil {
		t.Fatal("missing clientRequestId was accepted")
	}
	if _, _, err := NormalizeChatRequestIdentity(ChatRequest{ClientRequestID: "not-a-uuid", Message: "hello"}); err == nil {
		t.Fatal("invalid clientRequestId was accepted")
	}

	requestID := uuid.NewString()
	normalized, fingerprint, err := NormalizeChatRequestIdentity(ChatRequest{ClientRequestID: requestID, Message: " hello "})
	if err != nil || normalized.ClientRequestID != requestID || fingerprint == "" {
		t.Fatalf("NormalizeChatRequestIdentity = request:%+v fingerprint:%q err:%v", normalized, fingerprint, err)
	}
	_, changedFingerprint, err := NormalizeChatRequestIdentity(ChatRequest{ClientRequestID: requestID, Message: "different"})
	if err != nil || changedFingerprint == fingerprint {
		t.Fatalf("changed request fingerprint = %q err=%v, original=%q", changedFingerprint, err, fingerprint)
	}
	_, lowFingerprint, err := NormalizeChatRequestIdentity(ChatRequest{
		ClientRequestID: requestID, Message: "hello", ReasoningEffortOverride: ReasoningEffortLow,
	})
	if err != nil {
		t.Fatalf("low reasoning fingerprint: %v", err)
	}
	_, highFingerprint, err := NormalizeChatRequestIdentity(ChatRequest{
		ClientRequestID: requestID, Message: "hello", ReasoningEffortOverride: ReasoningEffortHigh,
	})
	if err != nil || highFingerprint == lowFingerprint {
		t.Fatalf("reasoning fingerprints low=%q high=%q err=%v", lowFingerprint, highFingerprint, err)
	}
}

func TestConcurrentResponsesRequestReusesOneRunAndNativeAssistantEvent(t *testing.T) {
	t.Run("responses", func(t *testing.T) {
		runtime := newTestRuntime(t)
		var calls atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			calls.Add(1)
			time.Sleep(40 * time.Millisecond)
			w.Header().Set("Content-Type", "text/event-stream")
			writeResponsesChatEvents(w)
		}))
		defer server.Close()

		baseURL := server.URL + "/v1"
		provider := mustSaveProvider(t, runtime, ProviderWriteRequest{
			ID: "idempotency-responses", DisplayName: "responses", BaseURL: baseURL, Model: "test-model",
			APIKey: "secret", Enabled: true,
		})
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "idempotency-agent-responses", Name: "responses", ProviderID: provider.ID, Model: provider.Model, Status: AgentStatusEnabled,
		})
		session := mustCreateSession(t, runtime, agent.ID, "idempotency")
		request := ChatRequest{ClientRequestID: uuid.NewString(), AgentID: agent.ID, SessionID: session.ID, Message: "hello"}

		responses := make([]ChatResponse, 2)
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for index := range responses {
			wg.Go(func() {
				responses[index], errs[index] = runtime.Chat(t.Context(), request)
			})
		}
		wg.Wait()
		for index, err := range errs {
			if err != nil {
				t.Fatalf("Chat[%d]: %v", index, err)
			}
		}
		if responses[0].Run.ID == "" || responses[0].Run.ID != responses[1].Run.ID || calls.Load() != 1 {
			t.Fatalf("responses run IDs=(%q,%q) model calls=%d", responses[0].Run.ID, responses[1].Run.ID, calls.Load())
		}

		run, ok, err := runtime.Store().Run(t.Context(), responses[0].Run.ID)
		if err != nil || !ok || run.FinalMessageID == "" {
			t.Fatalf("stored run ok=%v err=%v run=%+v", ok, err, run)
		}
		projection, ok, err := runtime.Store().SessionProjection(t.Context(), session.ID)
		if err != nil || !ok {
			t.Fatalf("SessionProjection ok=%v err=%v", ok, err)
		}
		message, found := projection.MessagesByEventID[run.FinalMessageID]
		if !found || message.Content != "single answer" || len(mustAssistantMessages(t, runtime, session.ID)) != 1 {
			t.Fatalf("native final message found=%v message=%+v projected=%+v", found, message, projection.Messages)
		}

		replayed, err := runtime.Chat(t.Context(), request)
		if err != nil || replayed.Run.ID != run.ID || calls.Load() != 1 {
			t.Fatalf("replayed response=%+v err=%v calls=%d", replayed, err, calls.Load())
		}
		conflicting := request
		conflicting.Message = "different"
		if _, err := runtime.Chat(t.Context(), conflicting); !errors.Is(err, ErrChatRequestConflict) {
			t.Fatalf("conflicting Chat err=%v", err)
		}
	})
}

func writeResponsesChatEvents(w http.ResponseWriter) {
	events := []map[string]any{
		{"type": "response.created", "response": map[string]any{"id": "resp-idempotent", "model": "test-model"}},
		{"type": "response.output_text.delta", "delta": "single answer"},
		{"type": "response.completed", "response": map[string]any{"id": "resp-idempotent", "model": "test-model", "usage": map[string]any{"total_tokens": 2}}},
	}
	for _, event := range events {
		payload, _ := json.Marshal(event)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}
