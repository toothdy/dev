package task

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseExpressionSupportsStructuredJSONArguments(t *testing.T) {
	expression, err := ParseExpression(`taskDemoService.test(1, "a,b", [1, 2], {"enabled": true, "nested": {"value": ")"}})`)
	if err != nil {
		t.Fatalf("parse expression failed: %v", err)
	}
	if expression.Key != "taskDemoService.test" || len(expression.Arguments) != 4 {
		t.Fatalf("unexpected expression: %#v", expression)
	}
	var object map[string]interface{}
	if err = json.Unmarshal(expression.Arguments[3], &object); err != nil {
		t.Fatalf("decode object argument failed: %v", err)
	}
	if object["enabled"] != true {
		t.Fatalf("unexpected object argument: %#v", object)
	}
}

func TestParseExpressionRejectsUnsafeOrMalformedInput(t *testing.T) {
	tests := []string{
		"taskDemoService.test",
		"taskDemoService.test(unknown)",
		"taskDemoService.test(1,) trailing",
		"taskDemoService.child.test()",
		"taskDemoService[\"test\"]()",
		strings.Repeat("a", MaxExpressionLength+1),
	}
	for _, value := range tests {
		if _, err := ParseExpression(value); err == nil {
			t.Fatalf("expected expression %q to fail", value)
		}
	}
}

func TestParseExpressionRejectsTooManyArguments(t *testing.T) {
	arguments := make([]string, MaxExpressionArguments+1)
	for index := range arguments {
		arguments[index] = "1"
	}
	if _, err := ParseExpression("taskDemoService.test(" + strings.Join(arguments, ",") + ")"); err == nil {
		t.Fatal("expected argument limit error")
	}
}
