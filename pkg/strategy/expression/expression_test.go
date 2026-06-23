package expression

import "testing"

func TestParseExpressionRejectsBlankInput(t *testing.T) {
	if _, err := ParseExpression("   "); err == nil {
		t.Fatal("ParseExpression(blank) error = nil")
	}
}

func TestParseExpressionParsesTrimmedExpressions(t *testing.T) {
	node, err := ParseExpression(" close + 1 ")
	if err != nil {
		t.Fatalf("ParseExpression: %v", err)
	}
	if node == nil {
		t.Fatal("ParseExpression returned nil node")
	}
}

func TestParseExpressionRejectsInvalidSyntax(t *testing.T) {
	if _, err := ParseExpression("close +"); err == nil {
		t.Fatal("ParseExpression(invalid) error = nil")
	}
}
