package pine

import "testing"

func TestParseExpressionRejectsBlankInput(t *testing.T) {
	if _, err := parseExpression("   "); err == nil {
		t.Fatal("parseExpression(blank) error = nil")
	}
}

func TestParseExpressionParsesTrimmedExpressions(t *testing.T) {
	node, err := parseExpression(" close + 1 ")
	if err != nil {
		t.Fatalf("parseExpression: %v", err)
	}
	if node == nil {
		t.Fatal("parseExpression returned nil node")
	}
}

func TestParseExpressionRejectsInvalidSyntax(t *testing.T) {
	if _, err := parseExpression("close +"); err == nil {
		t.Fatal("parseExpression(invalid) error = nil")
	}
}
