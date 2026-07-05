package pine

import (
	"strings"
	"testing"
)

func TestCompileRejectsInvalidObjectAndCollectionContracts(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		message string
	}{
		{
			name:    "nested type declaration",
			body:    "if close > open\n    type PriceBox\n        float price",
			message: "type declarations must be top level",
		},
		{name: "invalid type name", body: "type 123Price\n    float price", message: "invalid type name"},
		{
			name:    "duplicate type",
			body:    "type PriceBox\n    float price\ntype PriceBox\n    float other",
			message: "already declared",
		},
		{name: "type without fields", body: "type Empty", message: "requires at least one field"},
		{name: "invalid type field", body: "type PriceBox\n    float", message: "type field requires"},
		{
			name:    "duplicate type field",
			body:    "type PriceBox\n    float price\n    float price",
			message: "repeats field",
		},
		{
			name:    "nested method declaration",
			body:    "type PriceBox\n    float price\nif close > open\n    method score(PriceBox self) => self.price",
			message: "method declarations must be top level",
		},
		{
			name:    "method missing typed receiver",
			body:    "type PriceBox\n    float price\nmethod score(self) => close",
			message: "requires a typed receiver",
		},
		{
			name:    "method receiver type missing",
			body:    "method score(PriceBox self) => self.price",
			message: "receiver type PriceBox is not declared",
		},
		{
			name:    "method missing body",
			body:    "type PriceBox\n    float price\nmethod score(PriceBox self) =>",
			message: "requires one pure expression body",
		},
		{
			name:    "recursive method",
			body:    "type PriceBox\n    float price\nmethod score(PriceBox self) => self.score()",
			message: "side-effect-free and non-recursive",
		},
		{
			name:    "object field update requires reassignment",
			body:    "type PriceBox\n    float price\nbox = PriceBox.new(close)\nbox.price = open",
			message: "object field updates must use :=",
		},
		{
			name:    "unknown object field",
			body:    "type PriceBox\n    float price\nbox = PriceBox.new(close)\nbox.missing := open",
			message: "has no field missing",
		},
		{
			name:    "constructor positional overflow",
			body:    "type PriceBox\n    float price\nbox = PriceBox.new(close, open)",
			message: "PriceBox.new expects PriceBox.new(float price)",
		},
		{
			name:    "constructor unknown named field",
			body:    "type PriceBox\n    float price\nbox = PriceBox.new(missing=close)",
			message: "unknown constructor field missing",
		},
		{
			name:    "constructor duplicate named field",
			body:    "type PriceBox\n    float price\nbox = PriceBox.new(price=close, price=open)",
			message: "PriceBox.new expects PriceBox.new(float price)",
		},
		{
			name:    "constructor named overflow",
			body:    "type PriceBox\n    float price\nbox = PriceBox.new(price=close, open)",
			message: "PriceBox.new expects PriceBox.new(float price)",
		},
		{
			name: "method unknown named parameter",
			body: "type PriceBox\n    float price\nmethod score(PriceBox self, float factor, float offset=0) => self.price * factor + offset\n" +
				"box = PriceBox.new(close)\nvalue = box.score(missing=1)",
			message: "unknown method parameter missing",
		},
		{
			name: "method duplicate named parameter",
			body: "type PriceBox\n    float price\nmethod score(PriceBox self, float factor, float offset=0) => self.price * factor + offset\n" +
				"box = PriceBox.new(close)\nvalue = box.score(factor=1, factor=2)",
			message: "duplicate method argument factor",
		},
		{
			name: "method missing required parameter",
			body: "type PriceBox\n    float price\nmethod score(PriceBox self, float factor, float offset=0) => self.price * factor + offset\n" +
				"box = PriceBox.new(close)\nvalue = box.score(offset=2)",
			message: "method argument factor is required",
		},
		{
			name: "method positional overload mismatch",
			body: "type PriceBox\n    float price\nmethod score(PriceBox self, float factor, float offset=0) => self.price * factor + offset\n" +
				"box = PriceBox.new(close)\nvalue = box.score(1, 2, 3)",
			message: "box.score expects score(PriceBox self, float factor, float offset = 0)",
		},
		{
			name:    "typed collection needs constructor",
			body:    "array<float> values = close",
			message: "collection namespaces array/matrix/map are not executable",
		},
		{
			name:    "typed collection rejects read initializer",
			body:    "array<float> values = array.get(source, 0)",
			message: "collection namespaces array/matrix/map are not executable",
		},
		{
			name:    "typed collection namespace mismatch",
			body:    "array<float> values = map.new<string, float>()",
			message: "array declaration cannot be initialized with map.new",
		},
		{
			name:    "read call cannot be standalone",
			body:    "values = array.new_float(1, close)\narray.get(values, 0)",
			message: "collection namespaces array/matrix/map are not executable",
		},
		{
			name:    "namespace read requires target",
			body:    "value = array.get()",
			message: "array.get expects array.get(id, index)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			script := "//@version=6\nstrategy(\"Object collection rejection\", overlay=true)\n" + tc.body
			_, err := Compile(script)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Compile() error = %v, want message containing %q", err, tc.message)
			}
		})
	}
}
