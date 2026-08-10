package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

type workflowGraphDocument struct {
	Version   int                 `json:"version"`
	Engine    string              `json:"engine"`
	Objective string              `json:"objective"`
	Root      workflowGraphAgent  `json:"root"`
	Steps     []workflowGraphStep `json:"steps"`
}

type workflowGraphAgent struct {
	ID, Name, Instruction, ProviderID, Model, PermissionMode, WorkMode string
	LoopMaxIterations, RecentUserWindow                                int
	MemoryEnabled                                                      bool
	Tools, Skills                                                      []string
}

type workflowGraphStep struct {
	Order                                                                                int
	DependencyID, Title, Description, Message, AgentRole, ModeHint, Objective            string
	ChildAgentID, ChildProviderID, ChildModel, ChildPermissionMode, PlanSource, WorkMode string
	DependsOn                                                                            []string
	Agent                                                                                workflowGraphAgent
}

// WorkflowGraphFingerprint identifies the execution-relevant, resolved graph
// for one deterministic workflow run. Runtime status and output fields are
// deliberately excluded so pause/resume does not change the identity.
func WorkflowGraphFingerprint(parent Run, root Agent, steps []WorkflowStep, children []Agent) (string, error) {
	root = WorkflowGraphRootAgent(root, parent)
	document := workflowGraphDocument{
		Version: 1, Engine: strings.TrimSpace(parent.WorkflowEngine), Objective: strings.TrimSpace(parent.Objective),
		Root: workflowGraphAgentFrom(root), Steps: make([]workflowGraphStep, 0, len(steps)),
	}
	for index, step := range steps {
		child := root
		if index < len(children) {
			child = children[index]
		}
		document.Steps = append(document.Steps, workflowGraphStepFrom(step, child))
	}
	raw, _ := json.Marshal(document)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

// WorkflowGraphRootAgent applies the immutable run-time model and permission
// snapshot used by a workflow graph.
func WorkflowGraphRootAgent(agent Agent, parent Run) Agent {
	if value := strings.TrimSpace(parent.ProviderID); value != "" {
		agent.ProviderID = value
	}
	if value := strings.TrimSpace(parent.Model); value != "" {
		agent.Model = value
	}
	if ValidPermissionMode(parent.PermissionMode) {
		agent.PermissionMode = NormalizePermissionMode(parent.PermissionMode)
	}
	if value := strings.TrimSpace(parent.WorkMode); value != "" {
		agent.WorkMode = value
	}
	return agent
}

func workflowGraphAgentFrom(agent Agent) workflowGraphAgent {
	return workflowGraphAgent{
		ID: strings.TrimSpace(agent.ID), Name: strings.TrimSpace(agent.Name), Instruction: strings.TrimSpace(agent.Instruction),
		ProviderID: strings.TrimSpace(agent.ProviderID), Model: strings.TrimSpace(agent.Model),
		PermissionMode: NormalizePermissionMode(agent.PermissionMode), WorkMode: strings.TrimSpace(agent.WorkMode),
		LoopMaxIterations: agent.LoopMaxIterations, RecentUserWindow: agent.RecentUserWindow, MemoryEnabled: agent.MemoryEnabled,
		Tools: workflowGraphStrings(agent.Tools), Skills: workflowGraphStrings(agent.Skills),
	}
}

func workflowGraphStepFrom(step WorkflowStep, child Agent) workflowGraphStep {
	return workflowGraphStep{
		Order: step.Order, DependencyID: strings.TrimSpace(step.DependencyID), Title: strings.TrimSpace(step.Title),
		Description: strings.TrimSpace(step.Description), Message: strings.TrimSpace(step.Message), AgentRole: strings.TrimSpace(step.AgentRole),
		ModeHint: strings.TrimSpace(step.ModeHint), Objective: strings.TrimSpace(step.Objective), ChildAgentID: strings.TrimSpace(step.ChildAgentID),
		ChildProviderID: strings.TrimSpace(step.ChildProviderID), ChildModel: strings.TrimSpace(step.ChildModel),
		ChildPermissionMode: strings.TrimSpace(step.ChildPermissionMode), PlanSource: strings.TrimSpace(step.PlanSource),
		WorkMode: strings.TrimSpace(step.WorkflowMode), DependsOn: workflowGraphStrings(step.DependsOn), Agent: workflowGraphAgentFrom(child),
	}
}

func workflowGraphStrings(values []string) []string {
	unique := map[string]struct{}{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			unique[value] = struct{}{}
		}
	}
	out := make([]string, 0, len(unique))
	for value := range unique {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
