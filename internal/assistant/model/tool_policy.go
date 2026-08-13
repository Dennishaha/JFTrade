package model

import (
	"slices"
	"strings"
)

func ToolRequiredSkillNames(descriptor ToolDescriptor) []string {
	return NormalizeStringSlice(descriptor.RequiredSkills)
}

func ToolRequiresApproval(descriptor ToolDescriptor, mode string) bool {
	mode = NormalizePermissionMode(mode)
	if toolExplicitlySkipsApproval(descriptor.Name) {
		return false
	}
	if slices.Contains(descriptor.RequiresApprovalIn, mode) {
		return true
	}
	if mode == PermissionModeApproval && mediumOrHigherRisk(descriptor.RiskLevel) {
		return true
	}
	switch descriptor.Permission {
	case "install_skill", "write_strategy", "optimize_strategy", "write_task", "write_memory":
		return mode == PermissionModeApproval
	case "create_strategy_instance":
		return mode != PermissionModeAll
	case "live_trading":
		return true
	default:
		return false
	}
}

func mediumOrHigherRisk(risk string) bool {
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case "medium", "high", "critical":
		return true
	default:
		return false
	}
}

func toolExplicitlySkipsApproval(name string) bool {
	switch strings.TrimSpace(name) {
	case "tasks.create", "tasks.update", "tasks.delete", "memory.remember", "memory.forget", "strategy.save_draft", "strategy.research_backtest":
		return true
	default:
		return false
	}
}
