package schemaclean

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMergeOneOfAnyOf_PreservesType(t *testing.T) {
	v := []any{
		map[string]any{"type": "string", "description": "desc1"},
		map[string]any{"type": "integer", "description": "desc2"},
	}
	result := mergeOneOfAnyOf(v)

	if result["type"] != "string" {
		t.Fatalf("expected type 'string' (first non-object), got '%v'", result["type"])
	}
	desc := result["description"].(string)
	if !strings.Contains(desc, "desc1") || !strings.Contains(desc, "desc2") {
		t.Fatalf("expected merged descriptions, got '%s'", desc)
	}
}

func TestMergeOneOfAnyOf_MergesProperties(t *testing.T) {
	v := []any{
		map[string]any{"properties": map[string]any{"a": "prop_a"}},
		map[string]any{"properties": map[string]any{"b": "prop_b"}},
	}
	result := mergeOneOfAnyOf(v)

	props := result["properties"].(map[string]any)
	if props["a"] != "prop_a" || props["b"] != "prop_b" {
		t.Fatalf("expected merged properties, got %v", props)
	}
}

func TestMergeOneOfAnyOf_EmptyArray(t *testing.T) {
	result := mergeOneOfAnyOf([]any{})
	if result["type"] != "object" {
		t.Fatal("expected default type 'object' for empty oneOf/anyOf")
	}
	if _, ok := result["properties"]; !ok {
		t.Fatal("expected properties key in result")
	}
}

func TestMergeOneOfAnyOf_MergesEnum(t *testing.T) {
	v := []any{
		map[string]any{"enum": []any{"a", "b"}},
		map[string]any{"enum": []any{"b", "c"}},
	}
	result := mergeOneOfAnyOf(v)

	enums := result["enum"].([]any)
	if len(enums) != 3 { // a, b, c (deduped)
		t.Fatalf("expected 3 enum values after dedup, got %d", len(enums))
	}
}

func TestMergeOneOfAnyOf_MergesRequired(t *testing.T) {
	v := []any{
		map[string]any{"required": []any{"x"}},
		map[string]any{"required": []any{"y", "z"}},
	}
	result := mergeOneOfAnyOf(v)

	req := result["required"].([]any)
	if len(req) != 3 {
		t.Fatalf("expected 3 required fields, got %d", len(req))
	}
}

func TestForGemini_OneOfExpands(t *testing.T) {
	input := json.RawMessage(`{
		"type": "object",
		"properties": {},
		"oneOf": [
			{"type": "string", "description": "A string option"},
			{"type": "number", "description": "A number option"}
		]
	}`)
	result := ForGemini(input)

	var cleaned map[string]any
	if err := json.Unmarshal(result, &cleaned); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if _, ok := cleaned["oneOf"]; ok {
		t.Fatal("oneOf should have been removed")
	}
	if _, ok := cleaned["description"]; !ok {
		t.Fatal("expected description from oneOf merge")
	}
	if _, ok := cleaned["properties"]; !ok {
		t.Fatal("expected properties from oneOf expansion")
	}
}

func TestDedupeSlice(t *testing.T) {
	input := []any{"a", "b", "a", "c", "b"}
	result := dedupeSlice(input)
	if len(result) != 3 {
		t.Fatalf("expected 3 unique items, got %d", len(result))
	}
}

func TestForGemini_TypeUppercase(t *testing.T) {
	input := json.RawMessage(`{"type": "string"}`)
	result := ForGemini(input)

	var cleaned map[string]any
	json.Unmarshal(result, &cleaned)
	if cleaned["type"] != "STRING" {
		t.Fatalf("expected uppercase type, got %v", cleaned["type"])
	}
}
