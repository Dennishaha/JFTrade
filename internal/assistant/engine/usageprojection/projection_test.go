package usageprojection

import (
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"testing"

	"google.golang.org/genai"
)

func TestTrackerAccumulatesFinalUsageOnceAndPreservesHistory(t *testing.T) {
	historical := &assistantmodel.RunUsage{ModelCalls: 2, TokensIn: 10, TokensOut: 4}
	metadata := &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 7, CandidatesTokenCount: 5}
	var tracker Tracker

	usage, changed := tracker.Accumulate(" usage-event ", false, metadata, historical)
	if !changed || usage.ModelCalls != 3 || usage.TokensIn != 17 || usage.TokensOut != 9 {
		t.Fatalf("usage=%+v changed=%v, want calls=3 in=17 out=9", usage, changed)
	}
	if *historical != (assistantmodel.RunUsage{ModelCalls: 2, TokensIn: 10, TokensOut: 4}) {
		t.Fatalf("historical usage mutated through alias: %+v", historical)
	}
	if duplicate, duplicateChanged := tracker.Accumulate("usage-event", false, metadata, usage); duplicateChanged || duplicate != usage {
		t.Fatalf("duplicate usage=%+v changed=%v, want unchanged", duplicate, duplicateChanged)
	}
}

func TestTrackerIgnoresPartialMissingAndUnidentifiedEvents(t *testing.T) {
	metadata := &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 20, CandidatesTokenCount: 10}
	var tracker Tracker
	for _, test := range []struct {
		id       string
		partial  bool
		metadata *genai.GenerateContentResponseUsageMetadata
	}{{id: "partial", partial: true, metadata: metadata}, {metadata: metadata}, {id: "missing"}} {
		if usage, changed := tracker.Accumulate(test.id, test.partial, test.metadata, nil); changed || usage != nil {
			t.Fatalf("usage=%+v changed=%v, want ignored event", usage, changed)
		}
	}
}

func TestTrackerContinuesFromPersistedUsage(t *testing.T) {
	persisted := &assistantmodel.RunUsage{ModelCalls: 4, TokensIn: 100, TokensOut: 40}
	metadata := &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 25, CandidatesTokenCount: 9}
	usage, changed := new(Tracker).Accumulate("resumed-usage", false, metadata, persisted)
	if !changed || usage.ModelCalls != 5 || usage.TokensIn != 125 || usage.TokensOut != 49 {
		t.Fatalf("usage=%+v changed=%v, want calls=5 in=125 out=49", usage, changed)
	}
}
