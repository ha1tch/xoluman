// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package seedapply

import (
	"encoding/json"
	"errors"
	"testing"
)

// TestCreated_JSONKeysAreCamelCase guards against exactly the bug
// this test was written to catch: Created originally had no json
// tags at all, so it serialized with Go's default, capitalized field
// names ("Kind", "EntityType", ...) — invisible to any purely Go-side
// unit test, but a real mismatch against any JS consumer expecting
// xoluman's own established camelCase convention. Only surfaced by
// running the real UI against a real server and inspecting the actual
// wire bytes.
func TestCreated_JSONKeysAreCamelCase(t *testing.T) {
	data, err := json.Marshal(Created{Kind: CreatedEntity, EntityType: "companies", ID: 5, Key: "acme"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, want := range []string{"kind", "entityType", "id", "key"} {
		if _, present := m[want]; !present {
			t.Errorf("JSON output %s missing expected camelCase key %q", data, want)
		}
	}
	for _, notWant := range []string{"Kind", "EntityType", "ID", "Key"} {
		if _, present := m[notWant]; present {
			t.Errorf("JSON output %s still has the old, capitalized key %q", data, notWant)
		}
	}
}

// TestRollbackFailure_MarshalJSON_ErrorMessageSurvives guards the
// second half of the same bug class: Go's error interface has no
// exported fields, so json.Marshal's default struct encoding would
// silently produce {} for Err, losing the actual failure message
// entirely on the wire — exactly what made the rollback UI report
// "Removed 0 item(s)" with no visible reason during a real, live run.
func TestRollbackFailure_MarshalJSON_ErrorMessageSurvives(t *testing.T) {
	f := RollbackFailure{
		Created: Created{Kind: CreatedEntity, EntityType: "companies", ID: 7},
		Err:     errors.New("deliberate test error: connection refused"),
	}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	errMsg, _ := m["error"].(string)
	if errMsg != "deliberate test error: connection refused" {
		t.Fatalf("error field = %q, want the real error message preserved on the wire (got JSON: %s)", errMsg, data)
	}
	created, ok := m["created"].(map[string]any)
	if !ok {
		t.Fatalf("created field missing or wrong shape in %s", data)
	}
	if created["entityType"] != "companies" {
		t.Errorf("created.entityType = %v, want companies", created["entityType"])
	}
}

// TestRollbackReport_JSONKeysAreCamelCase confirms the full report
// shape end to end — every field a real UI reads (removed,
// removeFailures, notDeletable) present under its expected camelCase
// key, not the pre-fix capitalized Go default.
func TestRollbackReport_JSONKeysAreCamelCase(t *testing.T) {
	report := RollbackReport{
		Removed:        []Created{{Kind: CreatedEntity, ID: 1}},
		RemoveFailures: []RollbackFailure{{Created: Created{Kind: CreatedFSMDef, ID: 2}, Err: errors.New("boom")}},
		NotDeletable:   []Created{{Kind: CreatedDXPDef, ID: 3}},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, want := range []string{"removed", "removeFailures", "notDeletable"} {
		v, present := m[want]
		if !present {
			t.Fatalf("JSON output missing expected key %q: %s", want, data)
		}
		arr, ok := v.([]any)
		if !ok || len(arr) != 1 {
			t.Errorf("%s = %v, want a 1-element array", want, v)
		}
	}
}
