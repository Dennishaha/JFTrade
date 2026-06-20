package pineruntime

import (
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func buildHookCache(program *strategyir.Program) map[strategyir.HookKind]*strategyir.HookBlock {
	hooks := make(map[strategyir.HookKind]*strategyir.HookBlock)
	if program == nil {
		return hooks
	}
	for index := range program.Hooks {
		hook := &program.Hooks[index]
		hooks[hook.Kind] = hook
	}
	return hooks
}

func countProgramLetStatements(program *strategyir.Program) int {
	if program == nil {
		return 0
	}
	total := 0
	for _, hook := range program.Hooks {
		total += countLetStatements(hook.Statements)
	}
	return total
}

func countLetStatements(statements []strategyir.Statement) int {
	total := 0
	for _, statement := range statements {
		switch typed := statement.(type) {
		case *strategyir.LetStmt:
			total++
		case *strategyir.CollectionStmt:
			if typed.ResultName != "" {
				total++
			}
		case *strategyir.TupleStmt:
			total += len(typed.Names)
		case *strategyir.LoopStmt:
			total += countLetStatements(typed.Body)
		case *strategyir.ObjectStmt:
			if typed.ResultName != "" {
				total++
			}
		case *strategyir.IfStmt:
			total += countLetStatements(typed.Then)
			total += countLetStatements(typed.Else)
		}
	}
	return total
}

func buildIfScopePlans(program *strategyir.Program) map[*strategyir.IfStmt]ifScopePlan {
	if program == nil {
		return nil
	}
	plans := make(map[*strategyir.IfStmt]ifScopePlan)
	for _, hook := range program.Hooks {
		collectIfScopePlans(hook.Statements, plans)
	}
	return plans
}

func collectIfScopePlans(statements []strategyir.Statement, plans map[*strategyir.IfStmt]ifScopePlan) {
	for _, statement := range statements {
		if loop, ok := statement.(*strategyir.LoopStmt); ok {
			collectIfScopePlans(loop.Body, plans)
			continue
		}
		typed, ok := statement.(*strategyir.IfStmt)
		if !ok {
			continue
		}
		plans[typed] = ifScopePlan{
			thenNeedsClone: countLetStatements(typed.Then) > 0,
			elseNeedsClone: countLetStatements(typed.Else) > 0,
		}
		collectIfScopePlans(typed.Then, plans)
		collectIfScopePlans(typed.Else, plans)
	}
}
