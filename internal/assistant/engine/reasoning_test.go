package adk

import "testing"

func TestSplitAssistantContent(t *testing.T) {
	t.Parallel()

	reply, reasoning := splitLegacyAssistantContent("结论前 <think>这里是推理</think> 结论后")
	if reply != "结论前  结论后" {
		t.Fatalf("reply = %q, want %q", reply, "结论前  结论后")
	}
	if reasoning != "这里是推理" {
		t.Fatalf("reasoning = %q, want %q", reasoning, "这里是推理")
	}
}

func TestAssistantContentSplitterHandlesChunkedTags(t *testing.T) {
	t.Parallel()

	var splitter legacyAssistantContentSplitter
	reply1, reasoning1 := splitter.Push("先说<th")
	reply2, reasoning2 := splitter.Push("ink>推理")
	reply3, reasoning3 := splitter.Push("内容</think>结论")
	reply4, reasoning4 := splitter.Flush()

	if reply1 != "先说" || reasoning1 != "" {
		t.Fatalf("first chunk = (%q, %q)", reply1, reasoning1)
	}
	if reply2 != "" || reasoning2 != "推理" {
		t.Fatalf("second chunk = (%q, %q)", reply2, reasoning2)
	}
	if reply3 != "结论" || reasoning3 != "内容" {
		t.Fatalf("third chunk = (%q, %q)", reply3, reasoning3)
	}
	if reply4 != "" || reasoning4 != "" {
		t.Fatalf("flush = (%q, %q)", reply4, reasoning4)
	}
}

func TestExtractVisibleAndReasoningTextPrefersNativeReasoningFields(t *testing.T) {
	t.Parallel()

	reply, reasoning := extractVisibleAndReasoningText("最终答案", "第一段推理", "第二段推理")
	if reply != "最终答案" {
		t.Fatalf("reply = %q, want 最终答案", reply)
	}
	if reasoning != "第一段推理\n第二段推理" {
		t.Fatalf("reasoning = %q, want merged native reasoning", reasoning)
	}
}

func TestExtractVisibleAndReasoningTextPreservesChunkSpacing(t *testing.T) {
	t.Parallel()

	reply, reasoning := extractVisibleAndReasoningText(" me", " data")
	if reply != " me" {
		t.Fatalf("reply = %q, want leading-space chunk", reply)
	}
	if reasoning != " data" {
		t.Fatalf("reasoning = %q, want leading-space reasoning chunk", reasoning)
	}
}

func TestAssistantContentSplitterFlushesPartialAndCaseInsensitiveTags(t *testing.T) {
	t.Parallel()

	var splitter legacyAssistantContentSplitter
	reply, reasoning := splitter.Push("<THINK")
	if reply != "" || reasoning != "" {
		t.Fatalf("partial opening push = (%q, %q), want empty", reply, reasoning)
	}
	reply, reasoning = splitter.Flush()
	if reply != "<THINK" || reasoning != "" {
		t.Fatalf("partial opening flush = (%q, %q), want literal reply", reply, reasoning)
	}

	splitter = legacyAssistantContentSplitter{}
	if reply, reasoning = splitter.Push("<REASONING>"); reply != "" || reasoning != "" {
		t.Fatalf("case-insensitive opening tag push = (%q, %q)", reply, reasoning)
	}
	if reply, reasoning = splitter.Push("hidden"); reply != "" || reasoning != "hidden" {
		t.Fatalf("reasoning mode push = (%q, %q)", reply, reasoning)
	}
	if reply, reasoning = splitter.Push("</REASONING>shown"); reply != "shown" || reasoning != "" {
		t.Fatalf("case-insensitive closing tag push = (%q, %q)", reply, reasoning)
	}

	splitter = legacyAssistantContentSplitter{mode: reasoningModeReasoning}
	if reply, reasoning = splitter.Push("<not-a-reasoning-tag"); reply != "" || reasoning != "<not-a-reasoning-tag" {
		t.Fatalf("invalid tag in reasoning mode = (%q, %q)", reply, reasoning)
	}

	splitter = legacyAssistantContentSplitter{}
	if reply, reasoning = splitter.Push("<reasoning>"); reply != "" || reasoning != "" {
		t.Fatalf("opening reasoning tag push = (%q, %q)", reply, reasoning)
	}
	if reply, reasoning = splitter.Flush(); reply != "" || reasoning != "" {
		t.Fatalf("flush after consumed opening tag = (%q, %q)", reply, reasoning)
	}
}
