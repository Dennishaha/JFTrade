package adk

import (
	"fmt"
	"strings"

	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/workflowagents/loopagent"
	"google.golang.org/adk/v2/agent/workflowagents/parallelagent"
	"google.golang.org/adk/v2/agent/workflowagents/sequentialagent"
)

func newGoogleADKSequentialWorkflowAgent(name, description string, subAgents []adkagent.Agent) (adkagent.Agent, error) {
	cfg, err := googleADKFixedWorkflowAgentConfig(name, description, subAgents)
	if err != nil {
		return nil, err
	}
	return sequentialagent.New(sequentialagent.Config{AgentConfig: cfg})
}

func newGoogleADKParallelWorkflowAgent(name, description string, subAgents []adkagent.Agent) (adkagent.Agent, error) {
	cfg, err := googleADKFixedWorkflowAgentConfig(name, description, subAgents)
	if err != nil {
		return nil, err
	}
	return parallelagent.New(parallelagent.Config{AgentConfig: cfg})
}

func newGoogleADKLoopWorkflowAgent(name, description string, subAgents []adkagent.Agent, maxIterations uint) (adkagent.Agent, error) {
	if maxIterations == 0 {
		return nil, fmt.Errorf("fixed loop workflow agent %q requires max iterations", strings.TrimSpace(name))
	}
	cfg, err := googleADKFixedWorkflowAgentConfig(name, description, subAgents)
	if err != nil {
		return nil, err
	}
	return loopagent.New(loopagent.Config{AgentConfig: cfg, MaxIterations: maxIterations})
}

func googleADKFixedWorkflowAgentConfig(name, description string, subAgents []adkagent.Agent) (adkagent.Config, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return adkagent.Config{}, fmt.Errorf("fixed workflow agent name is required")
	}
	if len(subAgents) == 0 {
		return adkagent.Config{}, fmt.Errorf("fixed workflow agent %q requires at least one sub-agent", name)
	}
	return adkagent.Config{
		Name:        name,
		Description: strings.TrimSpace(description),
		SubAgents:   subAgents,
	}, nil
}
