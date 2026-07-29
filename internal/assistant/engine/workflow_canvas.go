package adk

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type WorkflowCanvasRunRequest struct {
	Workflow  WorkflowDefinition
	SessionID string
	Message   string
	Objective string
}

func (r *Runtime) RunCanvasWorkflow(ctx context.Context, req WorkflowCanvasRunRequest) (ChatResponse, error) {
	text, err := r.prepareChatRequest(ctx, ChatRequest{Message: req.Message})
	if err != nil {
		return ChatResponse{}, err
	}
	defer func() { <-r.runSem }()

	workflow := NormalizeWorkflowDefinition(req.Workflow)
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		objective = text
	}
	steps, err := compileWorkflowCanvasSteps(workflow, text, objective)
	if err != nil {
		return ChatResponse{}, err
	}

	agent, err := r.resolveAgentDefinition(ctx, workflow.AgentID)
	if err != nil {
		return ChatResponse{}, err
	}
	if providerID := strings.TrimSpace(workflow.ProviderID); providerID != "" {
		agent.ProviderID = providerID
	}
	if model := strings.TrimSpace(workflow.Model); model != "" {
		agent.Model = model
	}
	if permissionMode := strings.TrimSpace(workflow.PermissionMode); permissionMode != "" {
		if !validPermissionMode(permissionMode) {
			return ChatResponse{}, fmt.Errorf("invalid permission mode %q", permissionMode)
		}
		agent.PermissionMode = normalizePermissionMode(permissionMode)
	}
	agent.WorkMode = WorkModeLoop
	agent, err = r.resolveAgentProvider(ctx, agent)
	if err != nil {
		return ChatResponse{}, err
	}
	agent, err = r.prepareAgent(ctx, agent)
	if err != nil {
		return ChatResponse{}, err
	}
	session, err := r.resolveSession(ctx, req.SessionID, agent, text)
	if err != nil {
		return ChatResponse{}, err
	}
	if err := r.maybeAutoCompactSession(ctx, session, agent, text, nil); err != nil {
		return ChatResponse{}, err
	}

	executor := r.workflowExecutor()
	parent, parentCtx, finishParent, err := r.startRunWithOptions(ctx, session.ID, agent, text, runStartOptions{
		WorkMode:       WorkModeLoop,
		Objective:      objective,
		WorkflowStatus: workflowStatusRunning,
		WorkflowEngine: WorkflowEngineADK2Canvas,
	})
	if err != nil {
		return ChatResponse{}, err
	}
	defer finishParent()
	tasks, err := executor.persistWorkflowTasks(parentCtx, parent, agent, steps)
	if err != nil {
		parent, persistErr := executor.failParent(parentCtx, parent, err)
		if persistErr != nil {
			return ChatResponse{}, persistErr
		}
		return executor.workflowResponse(parentCtx, session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}
	parent.WorkflowPlan = workflowPlanFromTasks(tasks, parent.WorkflowPlan)
	if err := r.store.SaveRun(parentCtx, parent); err != nil {
		parent, persistErr := executor.failParent(parentCtx, parent, err)
		if persistErr != nil {
			return ChatResponse{}, persistErr
		}
		return executor.workflowResponse(parentCtx, session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}
	return executor.runPlannedGoogleADKWorkflow(parentCtx, workflowRequest{
		Agent: agent, Session: session, Message: text, Mode: WorkModeLoop, Objective: objective,
	}, parent, steps, tasks)
}

func compileWorkflowCanvasSteps(workflow WorkflowDefinition, message string, objective string) ([]workflowStep, error) {
	graph := workflow.CanvasGraph
	if graph == nil {
		return nil, fmt.Errorf("workflow canvas graph is required")
	}
	compiler := workflowCanvasCompiler{
		workflow:  workflow,
		graph:     graph,
		message:   strings.TrimSpace(message),
		objective: strings.TrimSpace(objective),
	}
	return compiler.compile()
}

type workflowCanvasCompiler struct {
	workflow  WorkflowDefinition
	graph     *WorkflowCanvasGraph
	message   string
	objective string
	nodes     map[string]WorkflowCanvasNode
	types     map[string]string
	out       map[string][]string
	in        map[string][]string
}

func (c *workflowCanvasCompiler) compile() ([]workflowStep, error) {
	if err := c.index(); err != nil {
		return nil, err
	}
	order, err := c.topologicalOrder()
	if err != nil {
		return nil, err
	}
	reachable := c.reachableNodeIDs()
	steps := make([]workflowStep, 0)
	for _, nodeID := range order {
		if c.types[nodeID] != "agent" {
			continue
		}
		if !reachable[nodeID] {
			return nil, fmt.Errorf("workflow canvas agent node %q is not reachable from start or trigger", nodeID)
		}
		node := c.nodes[nodeID]
		deps := c.upstreamAgentDependencies(nodeID)
		steps = append(steps, c.stepFromAgentNode(node, len(steps)+1, deps))
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("workflow canvas contains no executable agent nodes")
	}
	return steps, nil
}

func (c *workflowCanvasCompiler) index() error {
	c.nodes = map[string]WorkflowCanvasNode{}
	c.types = map[string]string{}
	c.out = map[string][]string{}
	c.in = map[string][]string{}
	for _, node := range c.graph.Nodes {
		id := strings.TrimSpace(node.ID)
		if id == "" {
			return fmt.Errorf("workflow canvas node id is required")
		}
		if _, exists := c.nodes[id]; exists {
			return fmt.Errorf("workflow canvas duplicate node id %q", id)
		}
		node.ID = id
		nodeType := workflowCanvasNodeType(node)
		switch nodeType {
		case "trigger", "start", "agent", "monitor":
		default:
			return fmt.Errorf("workflow canvas node %q has unsupported type %q", id, node.Type)
		}
		c.nodes[id] = node
		c.types[id] = nodeType
	}
	for _, edge := range c.graph.Edges {
		source := strings.TrimSpace(edge.Source)
		target := strings.TrimSpace(edge.Target)
		if source == "" || target == "" {
			return fmt.Errorf("workflow canvas edge %q requires source and target", edge.ID)
		}
		if source == target {
			return fmt.Errorf("workflow canvas edge %q must not connect a node to itself", edge.ID)
		}
		if _, ok := c.nodes[source]; !ok {
			return fmt.Errorf("workflow canvas edge %q references unknown source %q", edge.ID, source)
		}
		if _, ok := c.nodes[target]; !ok {
			return fmt.Errorf("workflow canvas edge %q references unknown target %q", edge.ID, target)
		}
		c.out[source] = append(c.out[source], target)
		c.in[target] = append(c.in[target], source)
	}
	return nil
}

func (c *workflowCanvasCompiler) topologicalOrder() ([]string, error) {
	indegree := make(map[string]int, len(c.nodes))
	for id := range c.nodes {
		indegree[id] = 0
	}
	for _, targets := range c.out {
		for _, target := range targets {
			indegree[target]++
		}
	}
	ready := make([]string, 0, len(indegree))
	for id, count := range indegree {
		if count == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	order := make([]string, 0, len(c.nodes))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		targets := append([]string(nil), c.out[id]...)
		sort.Strings(targets)
		for _, target := range targets {
			indegree[target]--
			if indegree[target] == 0 {
				ready = append(ready, target)
				sort.Strings(ready)
			}
		}
	}
	if len(order) != len(c.nodes) {
		return nil, fmt.Errorf("workflow canvas contains a cycle")
	}
	return order, nil
}

func (c *workflowCanvasCompiler) reachableNodeIDs() map[string]bool {
	roots := make([]string, 0)
	for id, nodeType := range c.types {
		if nodeType == "trigger" || nodeType == "start" {
			roots = append(roots, id)
		}
	}
	reachable := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if reachable[id] {
			return
		}
		reachable[id] = true
		for _, target := range c.out[id] {
			visit(target)
		}
	}
	for _, root := range roots {
		visit(root)
	}
	return reachable
}

func (c *workflowCanvasCompiler) upstreamAgentDependencies(nodeID string) []string {
	seen := map[string]bool{}
	var deps []string
	var visit func(string)
	visit = func(id string) {
		for _, source := range c.in[id] {
			if c.types[source] == "agent" {
				if !seen[source] {
					seen[source] = true
					deps = append(deps, source)
				}
				continue
			}
			visit(source)
		}
	}
	visit(nodeID)
	sort.Strings(deps)
	return deps
}

func (c *workflowCanvasCompiler) stepFromAgentNode(node WorkflowCanvasNode, order int, dependsOn []string) workflowStep {
	title := workflowCanvasNodeDataString(node, "title")
	if title == "" {
		title = workflowCanvasNodeDataString(node, "label")
	}
	if title == "" {
		title = node.ID
	}
	message := workflowCanvasNodeDataString(node, "message")
	if message == "" {
		message = workflowCanvasNodeDataString(node, "promptTemplate")
	}
	if message == "" {
		message = c.message
	}
	stepObjective := workflowCanvasNodeDataString(node, "objective")
	if stepObjective == "" {
		stepObjective = workflowCanvasNodeDataString(node, "objectiveTemplate")
	}
	if stepObjective == "" {
		stepObjective = c.objective
	}
	childAgentID := workflowCanvasNodeDataString(node, "agentId")
	if childAgentID == "" {
		childAgentID = c.workflow.AgentID
	}
	return workflowStep{
		Order:               order,
		DependencyID:        node.ID,
		Title:               title,
		Description:         workflowCanvasNodeDataString(node, "description"),
		Message:             message,
		DependsOn:           dependsOn,
		AgentRole:           workflowCanvasNodeDataString(node, "agentRole"),
		ChildAgentID:        childAgentID,
		ChildProviderID:     defaultString(workflowCanvasNodeDataString(node, "providerId"), c.workflow.ProviderID),
		ChildModel:          defaultString(workflowCanvasNodeDataString(node, "model"), c.workflow.Model),
		ChildPermissionMode: defaultString(workflowCanvasNodeDataString(node, "permissionMode"), c.workflow.PermissionMode),
		ModeHint:            WorkModeChat,
		Objective:           stepObjective,
		PlanSource:          workflowPlanSourceCanvas,
		WorkflowMode:        WorkModeLoop,
	}
}

func workflowCanvasNodeType(node WorkflowCanvasNode) string {
	nodeType := strings.ToLower(strings.TrimSpace(node.Type))
	if nodeType == "" {
		nodeType = strings.ToLower(strings.TrimSpace(workflowCanvasNodeDataString(node, "type")))
	}
	return nodeType
}

func workflowCanvasNodeDataString(node WorkflowCanvasNode, key string) string {
	if node.Data == nil {
		return ""
	}
	value, ok := node.Data[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
