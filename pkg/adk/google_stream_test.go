package adk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adkmodel "google.golang.org/adk/model"
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"
)

func TestOpenAICompatibleADKModelGenerateContentStreamYieldsPartialAndFinal(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, jftradeErr1 := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n"))
		jftradeCheckTestError(t, jftradeErr1)
		_, jftradeErr2 := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n"))
		jftradeCheckTestError(t, jftradeErr2)
		_, jftradeErr3 := w.Write([]byte("data: [DONE]\n\n"))
		jftradeCheckTestError(t, jftradeErr3)
	}))
	defer mockServer.Close()

	model := newOpenAICompatibleADKModel(Provider{
		BaseURL: strings.TrimSuffix(mockServer.URL, "/"),
		Model:   "gpt-test",
	}, "test-key", "gpt-test")

	req := &adkmodel.LLMRequest{
		Model:    "gpt-test",
		Contents: []*genai.Content{genai.NewContentFromText("你好", genai.RoleUser)},
	}

	var responses []*adkmodel.LLMResponse
	for response, err := range model.GenerateContent(context.Background(), req, true) {
		if err != nil {
			t.Fatalf("GenerateContent(stream=true) error: %v", err)
		}
		responses = append(responses, response)
	}

	if len(responses) != 3 {
		t.Fatalf("stream response count = %d, want 3", len(responses))
	}
	if !responses[0].Partial || responses[0].Content == nil || responses[0].Content.Parts[0].Text != "你" {
		t.Fatalf("partial[0] = %#v, want partial text 你", responses[0])
	}
	if !responses[1].Partial || responses[1].Content == nil || responses[1].Content.Parts[0].Text != "好" {
		t.Fatalf("partial[1] = %#v, want partial text 好", responses[1])
	}
	if responses[2].Partial {
		t.Fatalf("final response unexpectedly marked partial")
	}
	if got := responses[2].Content.Parts[0].Text; got != "你好" {
		t.Fatalf("final response text = %q, want 你好", got)
	}
}

func TestOpenAICompatibleADKModelGenerateContentStreamPreservesChunkSpacing(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, jftradeErr4 := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"Let\"}}]}\n\n"))
		jftradeCheckTestError(t, jftradeErr4)
		_, jftradeErr5 := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\" me\"}}]}\n\n"))
		jftradeCheckTestError(t, jftradeErr5)
		_, jftradeErr6 := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\" analyze\"}}]}\n\n"))
		jftradeCheckTestError(t, jftradeErr6)
		_, jftradeErr7 := w.Write([]byte("data: [DONE]\n\n"))
		jftradeCheckTestError(t, jftradeErr7)
	}))
	defer mockServer.Close()

	model := newOpenAICompatibleADKModel(Provider{
		BaseURL: strings.TrimSuffix(mockServer.URL, "/"),
		Model:   "gpt-test",
	}, "test-key", "gpt-test")

	req := &adkmodel.LLMRequest{
		Model:    "gpt-test",
		Contents: []*genai.Content{genai.NewContentFromText("analyze", genai.RoleUser)},
	}

	var final *adkmodel.LLMResponse
	for response, err := range model.GenerateContent(context.Background(), req, true) {
		if err != nil {
			t.Fatalf("GenerateContent(stream=true) error: %v", err)
		}
		if !response.Partial {
			final = response
		}
	}
	if final == nil || final.Content == nil || len(final.Content.Parts) == 0 {
		t.Fatalf("final response = %#v, want reasoning response", final)
	}
	if got := final.Content.Parts[0].Text; got != "Let me analyze" {
		t.Fatalf("final reasoning text = %q, want preserved spaces", got)
	}
}

func TestOpenAICompatibleADKModelGenerateContentStopsAfterYieldFalse(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, jftradeErr8 := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n"))
		jftradeCheckTestError(t, jftradeErr8)
		_, jftradeErr9 := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n"))
		jftradeCheckTestError(t, jftradeErr9)
		_, jftradeErr10 := w.Write([]byte("data: [DONE]\n\n"))
		jftradeCheckTestError(t, jftradeErr10)
	}))
	defer mockServer.Close()

	model := newOpenAICompatibleADKModel(Provider{
		BaseURL: strings.TrimSuffix(mockServer.URL, "/"),
		Model:   "gpt-test",
	}, "test-key", "gpt-test")

	req := &adkmodel.LLMRequest{
		Model:    "gpt-test",
		Contents: []*genai.Content{genai.NewContentFromText("你好", genai.RoleUser)},
	}

	count := 0
	for response, err := range model.GenerateContent(context.Background(), req, true) {
		if err != nil {
			t.Fatalf("GenerateContent(stream=true) unexpected error: %v", err)
		}
		if response == nil {
			t.Fatal("expected non-nil response before stopping iteration")
		}
		count++
		break
	}

	if count != 1 {
		t.Fatalf("response count = %d, want 1", count)
	}
}

func TestGoogleADKExecutionConsumeEventSkipsDuplicateFinalTextAfterPartial(t *testing.T) {
	execution := &googleADKExecution{}

	partialA := adksession.NewEvent("partial-a")
	partialA.LLMResponse = adkmodel.LLMResponse{
		Content: genai.NewContentFromText("你", genai.RoleModel),
		Partial: true,
	}
	partialB := adksession.NewEvent("partial-b")
	partialB.LLMResponse = adkmodel.LLMResponse{
		Content: genai.NewContentFromText("好", genai.RoleModel),
		Partial: true,
	}
	final := adksession.NewEvent("final")
	final.LLMResponse = adkmodel.LLMResponse{
		Content: genai.NewContentFromText("你好", genai.RoleModel),
	}

	if err := execution.consumeEvent(partialA); err != nil {
		t.Fatalf("consumeEvent(partialA): %v", err)
	}
	if err := execution.consumeEvent(partialB); err != nil {
		t.Fatalf("consumeEvent(partialB): %v", err)
	}
	if err := execution.consumeEvent(final); err != nil {
		t.Fatalf("consumeEvent(final): %v", err)
	}

	if got := execution.result().Reply; got != "你好" {
		t.Fatalf("reply = %q, want 你好", got)
	}
}
