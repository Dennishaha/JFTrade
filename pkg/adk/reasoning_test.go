package adk

import "testing"

func TestSplitAssistantContent(t *testing.T) {
	t.Parallel()

	reply, reasoning := splitAssistantContent("结论前 <think>这里是推理</think> 结论后")
	if reply != "结论前  结论后" {
		t.Fatalf("reply = %q, want %q", reply, "结论前  结论后")
	}
	if reasoning != "这里是推理" {
		t.Fatalf("reasoning = %q, want %q", reasoning, "这里是推理")
	}
}

func TestAssistantContentSplitterHandlesChunkedTags(t *testing.T) {
	t.Parallel()

	var splitter assistantContentSplitter
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
