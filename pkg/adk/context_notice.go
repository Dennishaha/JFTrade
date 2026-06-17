package adk

import (
	"context"
	"strings"
)

const (
	contextCompactionStartedText = "正在压缩上下文..."
	contextCompactionDoneText    = "已压缩上下文，继续使用最新摘要。"
	contextCompactionFailedText  = "上下文压缩失败，将继续使用当前上下文。"
)

func (r *Runtime) createContextCompactionNotice(ctx context.Context, sessionID string) TimelineEntry {
	return r.saveContextCompactionNotice(ctx, TimelineEntry{
		SessionID: strings.TrimSpace(sessionID),
		Kind:      TimelineKindContextNotice,
		Status:    TimelineStatusStreaming,
		Text:      contextCompactionStartedText,
	})
}

func (r *Runtime) updateContextCompactionNotice(ctx context.Context, notice TimelineEntry, status string, text string) TimelineEntry {
	if strings.TrimSpace(notice.ID) == "" {
		return TimelineEntry{}
	}
	notice.Status = strings.TrimSpace(status)
	notice.Text = strings.TrimSpace(text)
	return r.saveContextCompactionNotice(ctx, notice)
}

func (r *Runtime) saveContextCompactionNotice(ctx context.Context, notice TimelineEntry) TimelineEntry {
	if r == nil || r.store == nil || strings.TrimSpace(notice.SessionID) == "" {
		return TimelineEntry{}
	}
	saved, err := r.store.SaveSessionNotice(ctx, notice)
	if err != nil {
		return TimelineEntry{}
	}
	return saved
}

func emitContextCompactionNotice(onDelta func(ChatDelta) error, notice TimelineEntry) error {
	if onDelta == nil || strings.TrimSpace(notice.ID) == "" {
		return nil
	}
	notice = NormalizeTimelineEntry(notice)
	return onDelta(ChatDelta{Timeline: &notice})
}
