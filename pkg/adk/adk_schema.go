package adk

import (
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

func googleADKJSONSchemaFromMap(schema map[string]any) (*jsonschema.Schema, error) {
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
