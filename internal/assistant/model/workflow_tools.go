package model

import (
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	adkagent "google.golang.org/adk/v2/agent"
	adktool "google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
)

// SanitizeSchemaForOpenAI removes fields that many OpenAI-compatible providers
// reject (e.g. "additionalProperties": true) from a JSON Schema object.
func SanitizeSchemaForOpenAI(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	out := make(map[string]any, len(schema))
	for k, v := range schema {
		switch k {
		case "additionalProperties":
			// Many providers reject additionalProperties:true; omit the field entirely.
			if boolVal, ok := v.(bool); ok && boolVal {
				continue
			}
			out[k] = v
		case "properties":
			if nested, ok := v.(map[string]any); ok {
				sanitized := make(map[string]any, len(nested))
				for pk, pv := range nested {
					if sub, ok := pv.(map[string]any); ok {
						sanitized[pk] = SanitizeSchemaForOpenAI(sub)
					} else {
						sanitized[pk] = pv
					}
				}
				out[k] = sanitized
			} else {
				out[k] = v
			}
		case "items":
			if sub, ok := v.(map[string]any); ok {
				out[k] = SanitizeSchemaForOpenAI(sub)
			} else {
				out[k] = v
			}
		default:
			out[k] = v
		}
	}
	return out
}

// GoogleADKJSONSchemaFromMap converts a plain map into the GO-ADK JSON schema
// shape used by function tools.
func GoogleADKJSONSchemaFromMap(schema map[string]any) (*jsonschema.Schema, error) {
	if schema == nil {
		return nil, nil
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("encode GO-ADK JSON schema: %w", err)
	}
	var converted jsonschema.Schema
	if err := json.Unmarshal(raw, &converted); err != nil {
		return nil, fmt.Errorf("decode GO-ADK JSON schema: %w", err)
	}
	return &converted, nil
}

// WorkflowMapToolSpec describes one declarative workflow map function tool.
type WorkflowMapToolSpec struct {
	Name        string
	Description string
	Schema      map[string]any
	Run         func(map[string]any) (map[string]any, error)
}

// NewWorkflowMapFunctionTools builds multiple workflow map function tools.
func NewWorkflowMapFunctionTools(specs ...WorkflowMapToolSpec) ([]adktool.Tool, error) {
	tools := make([]adktool.Tool, 0, len(specs))
	for _, spec := range specs {
		created, err := NewWorkflowMapFunctionTool(spec)
		if err != nil {
			return nil, err
		}
		tools = append(tools, created)
	}
	return tools, nil
}

// NewWorkflowMapFunctionTool builds one declarative workflow map function tool.
func NewWorkflowMapFunctionTool(spec WorkflowMapToolSpec) (adktool.Tool, error) {
	schema, err := GoogleADKJSONSchemaFromMap(SanitizeSchemaForOpenAI(spec.Schema))
	if err != nil {
		return nil, fmt.Errorf("convert workflow tool schema %q: %w", spec.Name, err)
	}
	return functiontool.New[map[string]any, map[string]any](functiontool.Config{
		Name:        spec.Name,
		Description: spec.Description,
		InputSchema: schema,
	}, func(_ adkagent.Context, args map[string]any) (map[string]any, error) {
		if spec.Run == nil {
			return nil, fmt.Errorf("workflow tool %s is unavailable", spec.Name)
		}
		return spec.Run(args)
	})
}
