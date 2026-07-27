package strategy

import (
	"strings"

	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

const (
	RuntimePinePlan = pineworker.RuntimeID
	DefaultVersion  = "0.1.0"
)

// DefaultPine returns the canonical starter Pine v6 strategy source used by
// both definition and catalog normalization.
func DefaultPine(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Pine Strategy"
	}
	escapedName := strings.ReplaceAll(name, `"`, `\"`)
	return "//@version=6\n" +
		"strategy(\"" + escapedName + "\", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)\n\n" +
		"// JFTrade executes supported Pine strategy statements on each closed K line.\n" +
		"fast = ta.ema(close, 8)\n" +
		"slow = ta.ema(close, 21)\n" +
		"if ta.crossover(fast, slow)\n" +
		"    strategy.entry(\"Long\", strategy.long)\n"
}
