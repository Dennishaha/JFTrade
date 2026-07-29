package adk

import (
	"encoding/json"
	"testing"
)

func TestGoogleADKJSONSchemaFromMapPreservesObjectFields(t *testing.T) {
	schema, err := googleADKJSONSchemaFromMap(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"providerId": map[string]any{"type": "string"},
			"limit":      map[string]any{"type": "integer", "minimum": 1, "maximum": 100},
		},
		"additionalProperties": false,
	})
	if err != nil {
		t.Fatalf("googleADKJSONSchemaFromMap: %v", err)
	}
	if schema == nil {
		t.Fatalf("schema is nil")
		return
	}
	if schema.Type != "object" {
		t.Fatalf("schema type = %q, want object", schema.Type)
	}
	if _, ok := schema.Properties["providerId"]; !ok {
		t.Fatalf("schema properties = %+v, want providerId", schema.Properties)
	}
	if schema.AdditionalProperties == nil {
		t.Fatalf("additionalProperties = %+v, want false boolean schema", schema.AdditionalProperties)
	}
	additionalProperties, err := json.Marshal(schema.AdditionalProperties)
	if err != nil {
		t.Fatalf("marshal additionalProperties: %v", err)
	}
	if string(additionalProperties) != "false" {
		t.Fatalf("additionalProperties = %s, want false", additionalProperties)
	}
}

func TestGoogleADKJSONSchemaFromMapAllowsNil(t *testing.T) {
	schema, err := googleADKJSONSchemaFromMap(nil)
	if err != nil {
		t.Fatalf("googleADKJSONSchemaFromMap(nil): %v", err)
	}
	if schema != nil {
		t.Fatalf("schema = %+v, want nil", schema)
	}
}
