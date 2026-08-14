package productfeatures

import (
	"fmt"
	"strings"
)

func normalizeCandleOptions(sessions []string, adjustment string) ([]string, string, error) {
	for index := range sessions {
		sessions[index] = strings.ToLower(strings.TrimSpace(sessions[index]))
		switch sessions[index] {
		case "regular", "extended", "overnight":
		default:
			return nil, "", fmt.Errorf("%w: unsupported session %q", ErrInvalidQuery, sessions[index])
		}
	}
	adjustment = strings.ToLower(strings.TrimSpace(adjustment))
	if adjustment == "" {
		adjustment = "none"
	}
	switch adjustment {
	case "none", "forward", "backward":
		return sessions, adjustment, nil
	default:
		return nil, "", fmt.Errorf("%w: unsupported adjustment %q", ErrInvalidQuery, adjustment)
	}
}
