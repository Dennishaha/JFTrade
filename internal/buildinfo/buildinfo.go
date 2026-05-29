package buildinfo

import (
	"runtime"
	"strings"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = ""
)

func Snapshot() map[string]any {
	buildTime := strings.TrimSpace(BuildTime)
	if buildTime == "" {
		buildTime = "dev"
	}

	return map[string]any{
		"version":   strings.TrimSpace(Version),
		"commit":    strings.TrimSpace(Commit),
		"buildTime": buildTime,
		"goos":      runtime.GOOS,
		"goarch":    runtime.GOARCH,
	}
}
