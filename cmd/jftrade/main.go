// Command jftrade wraps the bbgo CLI with non-invasive plugins (Futu exchange).
package main

import (
	"github.com/c9s/bbgo/pkg/cmd"

	// Side-effect imports register plugins with bbgo at init() time.
	_ "github.com/jftrade/jftrade-main/pkg/futu"
)

func main() {
	cmd.Execute()
}
