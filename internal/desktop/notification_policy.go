package desktop

import (
	"strings"

	"github.com/jftrade/jftrade-main/internal/live"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

// ShouldForwardSystemNotification reports whether a live notification should
// be forwarded to the host OS notification center.
func ShouldForwardSystemNotification(settings jfsettings.SystemNotificationSettings, event live.Event) bool {
	if !settings.Enabled {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(settings.Mode)) {
	case "all":
		return true
	case "custom", "important":
		return matchesAny(normalize(event.Level), settings.Levels) || matchesAny(strings.TrimSpace(event.Category), settings.Categories)
	default:
		return false
	}
}

func NotificationThreadID(event live.Event) string {
	category := strings.TrimSpace(event.Category)
	if category != "" {
		return category
	}
	source := strings.TrimSpace(event.Source)
	if source != "" {
		return source
	}
	return "jftrade.system"
}

func NotificationInterruptionLevel(level string) string {
	switch normalize(level) {
	case "error":
		return "timeSensitive"
	case "warn":
		return "active"
	default:
		return "passive"
	}
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func matchesAny(value string, candidates []string) bool {
	if value == "" {
		return false
	}
	for _, candidate := range candidates {
		if value == strings.TrimSpace(candidate) || normalize(value) == normalize(candidate) {
			return true
		}
	}
	return false
}
