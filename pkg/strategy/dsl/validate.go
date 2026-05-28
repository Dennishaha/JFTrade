package dsl

import strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"

const (
	hookInit       = "on init:"
	hookKLineClose = "on kline_close:"
)

func ValidateScript(script string) error {
	program, err := ParseScript(script)
	if err != nil {
		return err
	}
	_, err = strategyir.PlanRequirements(program)
	return err
}
