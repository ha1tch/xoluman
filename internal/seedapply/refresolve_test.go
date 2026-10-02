// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package seedapply

import "testing"

func TestParseJSONLEntities_Basic(t *testing.T) {
	data := []byte(`{"_key":"a","name":"Acme"}
{"_key":"b","name":"Beta"}
`)
	entities, err := ParseJSONLEntities(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 2 {
		t.Fatalf("got %d entities, want 2", len(entities))
	}
	if entities[0].Key != "a" || entities[0].Fields["name"] != "Acme" {
		t.Errorf("entity 0 = %+v, want key=a name=Acme", entities[0])
	}
	if entities[1].Key != "b" || entities[1].Fields["name"] != "Beta" {
		t.Errorf("entity 1 = %+v, want key=b name=Beta", entities[1])
	}
}

func TestParseJSONLEntities_KeyStrippedFromFields(t *testing.T) {
	entities, err := ParseJSONLEntities([]byte(`{"_key":"a","name":"Acme"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, present := entities[0].Fields["_key"]; present {
		t.Error("_key should be removed from Fields, still present")
	}
}

func TestParseJSONLEntities_NoKeyIsValid(t *testing.T) {
	entities, err := ParseJSONLEntities([]byte(`{"name":"Acme"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entities[0].Key != "" {
		t.Errorf("Key = %q, want empty for a record with no _key", entities[0].Key)
	}
}

func TestParseJSONLEntities_BlankLinesSkipped(t *testing.T) {
	data := []byte("{\"_key\":\"a\",\"name\":\"Acme\"}\n\n   \n{\"_key\":\"b\",\"name\":\"Beta\"}\n")
	entities, err := ParseJSONLEntities(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 2 {
		t.Fatalf("got %d entities, want 2 (blank lines should be skipped)", len(entities))
	}
}

func TestParseJSONLEntities_InvalidJSONLine(t *testing.T) {
	_, err := ParseJSONLEntities([]byte(`{"_key":"a","name":"Acme"}` + "\n" + `not json at all`))
	if err == nil {
		t.Fatal("expected an error for an invalid JSON line, got none")
	}
}

func TestParseJSONLEntities_NonStringKeyRejected(t *testing.T) {
	_, err := ParseJSONLEntities([]byte(`{"_key":123,"name":"Acme"}`))
	if err == nil {
		t.Fatal("expected an error for a non-string _key, got none")
	}
}

func TestParseJSONLEntities_Empty(t *testing.T) {
	entities, err := ParseJSONLEntities([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0 for empty input", len(entities))
	}
}

func TestSplitFields_NoRefs(t *testing.T) {
	immediate, deferred := SplitFields(map[string]any{"name": "Acme", "size": float64(50)})
	if len(deferred) != 0 {
		t.Errorf("deferred = %v, want empty", deferred)
	}
	if immediate["name"] != "Acme" || immediate["size"] != float64(50) {
		t.Errorf("immediate = %v, want both fields passed through", immediate)
	}
}

func TestSplitFields_OneRef(t *testing.T) {
	fields := map[string]any{
		"name":  "Big Deal",
		"owner": map[string]any{"$ref": "user-alice"},
	}
	immediate, deferred := SplitFields(fields)
	if _, present := immediate["owner"]; present {
		t.Error("owner should be deferred, not immediate")
	}
	if immediate["name"] != "Big Deal" {
		t.Errorf("immediate[name] = %v, want Big Deal", immediate["name"])
	}
	if deferred["owner"] != "user-alice" {
		t.Errorf("deferred[owner] = %q, want user-alice", deferred["owner"])
	}
}

func TestSplitFields_MultipleRefs(t *testing.T) {
	fields := map[string]any{
		"owner":   map[string]any{"$ref": "user-alice"},
		"company": map[string]any{"$ref": "company-acme"},
		"amount":  float64(5000),
	}
	immediate, deferred := SplitFields(fields)
	if len(deferred) != 2 {
		t.Fatalf("deferred = %v, want 2 entries", deferred)
	}
	if len(immediate) != 1 || immediate["amount"] != float64(5000) {
		t.Errorf("immediate = %v, want only amount", immediate)
	}
}

func TestSplitFields_ObjectWithMultipleKeysIsNotARef(t *testing.T) {
	// {"$ref": "x", "extra": "y"} is NOT a $ref marker -- it has two
	// keys, not exactly one -- and must be treated as literal data,
	// not silently misread as a reference.
	fields := map[string]any{
		"weird": map[string]any{"$ref": "x", "extra": "y"},
	}
	immediate, deferred := SplitFields(fields)
	if len(deferred) != 0 {
		t.Errorf("deferred = %v, want empty -- a 2-key object is not a $ref marker", deferred)
	}
	if _, present := immediate["weird"]; !present {
		t.Error("the 2-key object should pass through as literal immediate data")
	}
}

func TestSplitFields_RefValueMustBeString(t *testing.T) {
	fields := map[string]any{
		"owner": map[string]any{"$ref": float64(5)},
	}
	immediate, deferred := SplitFields(fields)
	if len(deferred) != 0 {
		t.Errorf("deferred = %v, want empty -- a non-string $ref value is not a valid marker", deferred)
	}
	if _, present := immediate["owner"]; !present {
		t.Error("a non-string $ref should pass through as literal immediate data")
	}
}

func TestSplitFields_PlainObjectIsNotAref(t *testing.T) {
	fields := map[string]any{
		"address": map[string]any{"city": "Austin", "state": "TX"},
	}
	immediate, deferred := SplitFields(fields)
	if len(deferred) != 0 {
		t.Errorf("deferred = %v, want empty for an ordinary nested object", deferred)
	}
	if _, present := immediate["address"]; !present {
		t.Error("an ordinary nested object should pass through as immediate data")
	}
}
