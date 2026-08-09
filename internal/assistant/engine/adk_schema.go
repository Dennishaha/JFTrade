package adk

import (
	"github.com/google/jsonschema-go/jsonschema"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func googleADKJSONSchemaFromMap(schema map[string]any) (*jsonschema.Schema, error) {
	return jfadkmodel.GoogleADKJSONSchemaFromMap(schema)
}
