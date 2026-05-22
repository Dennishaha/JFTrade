package jftradeapi

import (
	"fmt"
	"strings"
	"time"
)

func liveNotificationFromBBGONotify(obj any, args ...any) *liveNotification {
	if obj == nil {
		return nil
	}
	if _, ok := obj.(forwardedBBGONotification); ok {
		return nil
	}

	note := liveNotification{
		At:       time.Now().UTC().Format(time.RFC3339Nano),
		Level:    "info",
		Title:    "BBGO 通知",
		Source:   "bbgo.notify",
		Category: "bbgo.notify",
	}

	switch value := obj.(type) {
	case error:
		note.Level = "error"
		note.Title = "BBGO 错误"
		note.Message = strings.TrimSpace(value.Error())
	default:
		text := strings.TrimSpace(formatBBGONotifyText(obj, args...))
		if text == "" {
			return nil
		}
		note.Level = inferBBGONotificationLevel(text)
		note.Message = text
	}

	if note.Message == "" {
		return nil
	}
	return &note
}

func formatBBGONotifyText(obj any, args ...any) string {
	switch value := obj.(type) {
	case string:
		if len(args) == 0 {
			return value
		}
		formatted := fmt.Sprintf(value, args...)
		if formatted != value {
			return formatted
		}
		return strings.TrimSpace(value + " " + joinNotifyArgs(args...))
	case fmt.Stringer:
		if len(args) == 0 {
			return value.String()
		}
		return strings.TrimSpace(value.String() + " " + joinNotifyArgs(args...))
	default:
		if len(args) == 0 {
			return fmt.Sprint(obj)
		}
		return strings.TrimSpace(fmt.Sprint(obj) + " " + joinNotifyArgs(args...))
	}
}

func joinNotifyArgs(args ...any) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, strings.TrimSpace(fmt.Sprint(arg)))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func inferBBGONotificationLevel(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "panic"),
		strings.Contains(lower, "fatal"),
		strings.Contains(lower, "error"),
		strings.Contains(lower, "failed"),
		strings.Contains(lower, "timeout"),
		strings.Contains(text, "失败"),
		strings.Contains(text, "错误"),
		strings.Contains(text, "超时"):
		return "error"
	case strings.Contains(lower, "warn"),
		strings.Contains(lower, "risk"),
		strings.Contains(lower, "retry"),
		strings.Contains(text, "警告"),
		strings.Contains(text, "告警"),
		strings.Contains(text, "风险"):
		return "warn"
	default:
		return "info"
	}
}
