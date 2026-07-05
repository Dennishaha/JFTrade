package adk

import (
	"fmt"
	"strings"

	adkworkflow "google.golang.org/adk/v2/workflow"
)

type workflowCompiler struct{}

func newWorkflowCompiler() workflowCompiler {
	return workflowCompiler{}
}

func (workflowCompiler) CompileEdges(steps []workflowStep, nodes []adkworkflow.Node) ([]adkworkflow.Edge, error) {
	edges := make([]adkworkflow.Edge, 0, len(nodes)*2)
	nodeByStepID := make(map[string]adkworkflow.Node, len(nodes))
	for index, node := range nodes {
		if index >= len(steps) {
			break
		}
		if stepID := strings.TrimSpace(steps[index].DependencyID); stepID != "" {
			nodeByStepID[stepID] = node
		}
	}
	for index, node := range nodes {
		if index >= len(steps) {
			break
		}
		dependencies, err := compileGoogleADKWorkflowDependencies(steps[index], nodeByStepID)
		if err != nil {
			return nil, err
		}
		switch len(dependencies) {
		case 0:
			edges = append(edges, adkworkflow.Edge{From: adkworkflow.Start, To: node})
		case 1:
			edges = append(edges, adkworkflow.Edge{From: dependencies[0], To: node})
		default:
			join := adkworkflow.NewJoinNode(fmt.Sprintf("%s_join", node.Name()))
			for _, dep := range dependencies {
				edges = append(edges, adkworkflow.Edge{From: dep, To: join})
			}
			edges = append(edges, adkworkflow.Edge{From: join, To: node})
		}
	}
	if len(edges) == 0 && len(nodes) > 0 {
		edges = append(edges, adkworkflow.Edge{From: adkworkflow.Start, To: nodes[0]})
	}
	return edges, nil
}

func compileGoogleADKWorkflowDependencies(step workflowStep, nodeByStepID map[string]adkworkflow.Node) ([]adkworkflow.Node, error) {
	if len(step.DependsOn) == 0 {
		return nil, nil
	}
	dependencies := make([]adkworkflow.Node, 0, len(step.DependsOn))
	seen := make(map[string]struct{}, len(step.DependsOn))
	for _, dependencyID := range step.DependsOn {
		dependencyID = strings.TrimSpace(dependencyID)
		if dependencyID == "" {
			continue
		}
		if _, ok := seen[dependencyID]; ok {
			continue
		}
		node, ok := nodeByStepID[dependencyID]
		if !ok || node == nil {
			return nil, fmt.Errorf("compile GO-ADK workflow dependencies for step %q: unknown dependency %q", step.DependencyID, dependencyID)
		}
		seen[dependencyID] = struct{}{}
		dependencies = append(dependencies, node)
	}
	return dependencies, nil
}
