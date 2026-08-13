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

type Information struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
}

func Snapshot() Information {
	buildTime := strings.TrimSpace(BuildTime)
	if buildTime == "" {
		buildTime = "dev"
	}

	return Information{
		Version:   strings.TrimSpace(Version),
		Commit:    strings.TrimSpace(Commit),
		BuildTime: buildTime,
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
	}
}
