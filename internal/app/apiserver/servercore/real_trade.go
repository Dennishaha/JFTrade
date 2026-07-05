package servercore

import (
	"os"
	"path/filepath"
	"strings"
)

const defaultRealTradeControlFilename = "real-trade-control.json"

func deriveRealTradeControlPath(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_REAL_TRADE_CONTROL_PATH")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultRealTradeControlFilename
	}
	return filepath.Join(directory, defaultRealTradeControlFilename)
}
