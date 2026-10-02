// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package seeds

import (
	"strings"
	"testing"
)

func validManifestJSON(stepsJSON string) string {
	return `{
		"format_version": 1,
		"id": "test-seed",
		"name": "Test Seed",
		"description": "a seed for testing",
		"tags": ["test"],
		"manual": "README.md",
		"preview_images": ["preview/a.png"],
		"steps": [` + stepsJSON + `]
	}`
}

func TestParseManifest_MinimalValid(t *testing.T) {
	data := []byte(`{"format_version":1,"id":"x","name":"X","steps":[{"type":"fsm","file":"fsms/a.json"}]}`)
	m, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID != "x" || m.Name != "X" || len(m.Steps) != 1 {
		t.Fatalf("got %+v, want id=x name=X 1 step", m)
	}
}

func TestParseManifest_FullValid(t *testing.T) {
	data := []byte(validManifestJSON(`
		{"type":"schema","entity_type":"companies","file":"schemas/companies.json"},
		{"type":"data","entity_type":"companies","file":"data/companies.jsonl"},
		{"type":"fsm","file":"fsms/approval.json"},
		{"type":"dxp_def","file":"dxp/close-deal.json"},
		{"type":"bal_define","file":"bal/accounts.json"},
		{"type":"bal_transfer","file":"bal/transfers.json"},
		{"type":"cal_create_calendar","file":"cal/calendars.json"},
		{"type":"cal_propose","file":"cal/bookings.json"},
		{"type":"raw","method":"POST","path":"/ts/events/batch","file":"raw/ts-events.json"}
	`))
	m, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Steps) != 9 {
		t.Fatalf("got %d steps, want 9", len(m.Steps))
	}
	if m.Manual != "README.md" {
		t.Errorf("Manual = %q, want README.md", m.Manual)
	}
	if len(m.PreviewImages) != 1 {
		t.Errorf("PreviewImages = %v, want 1 entry", m.PreviewImages)
	}
}

func TestParseManifest_MalformedJSON(t *testing.T) {
	_, err := ParseManifest([]byte(`{not valid json`))
	if err == nil {
		t.Fatal("expected an error for malformed JSON, got none")
	}
}

func TestParseManifest_WrongFormatVersion(t *testing.T) {
	data := []byte(`{"format_version":99,"id":"x","name":"X","steps":[{"type":"fsm","file":"a.json"}]}`)
	_, err := ParseManifest(data)
	if err == nil {
		t.Fatal("expected an error for an unsupported format_version, got none")
	}
}

func TestParseManifest_MissingID(t *testing.T) {
	data := []byte(`{"format_version":1,"name":"X","steps":[{"type":"fsm","file":"a.json"}]}`)
	_, err := ParseManifest(data)
	if err == nil {
		t.Fatal("expected an error for a missing id, got none")
	}
}

func TestParseManifest_MissingName(t *testing.T) {
	data := []byte(`{"format_version":1,"id":"x","steps":[{"type":"fsm","file":"a.json"}]}`)
	_, err := ParseManifest(data)
	if err == nil {
		t.Fatal("expected an error for a missing name, got none")
	}
}

func TestParseManifest_NoSteps(t *testing.T) {
	data := []byte(`{"format_version":1,"id":"x","name":"X","steps":[]}`)
	_, err := ParseManifest(data)
	if err == nil {
		t.Fatal("expected an error for zero steps, got none")
	}
}

func TestParseManifest_UnknownStepType(t *testing.T) {
	data := []byte(validManifestJSON(`{"type":"something_invented","file":"a.json"}`))
	_, err := ParseManifest(data)
	if err == nil {
		t.Fatal("expected an error for an unknown step type, got none")
	}
}

func TestParseManifest_StepMissingType(t *testing.T) {
	data := []byte(validManifestJSON(`{"file":"a.json"}`))
	_, err := ParseManifest(data)
	if err == nil {
		t.Fatal("expected an error for a step with no type, got none")
	}
}

func TestParseManifest_StepMissingFile(t *testing.T) {
	data := []byte(validManifestJSON(`{"type":"fsm"}`))
	_, err := ParseManifest(data)
	if err == nil {
		t.Fatal("expected an error for a step with no file, got none")
	}
}

func TestParseManifest_SchemaStepMissingEntityType(t *testing.T) {
	data := []byte(validManifestJSON(`{"type":"schema","file":"schemas/x.json"}`))
	_, err := ParseManifest(data)
	if err == nil {
		t.Fatal("expected an error for a schema step with no entity_type, got none")
	}
}

func TestParseManifest_DataStepMissingEntityType(t *testing.T) {
	data := []byte(validManifestJSON(`{"type":"data","file":"data/x.jsonl"}`))
	_, err := ParseManifest(data)
	if err == nil {
		t.Fatal("expected an error for a data step with no entity_type, got none")
	}
}

func TestParseManifest_NonEntityStepDoesNotRequireEntityType(t *testing.T) {
	// fsm, dxp_def, bal_*, cal_*, and raw steps must NOT require
	// entity_type -- only schema and data steps carry that field at
	// all, per requiresEntityType.
	data := []byte(validManifestJSON(`{"type":"fsm","file":"fsms/a.json"}`))
	if _, err := ParseManifest(data); err != nil {
		t.Fatalf("fsm step should not require entity_type, got error: %v", err)
	}
}

func TestParseManifest_RawStepMissingMethod(t *testing.T) {
	data := []byte(validManifestJSON(`{"type":"raw","path":"/ts/events","file":"raw/a.json"}`))
	_, err := ParseManifest(data)
	if err == nil {
		t.Fatal("expected an error for a raw step with no method, got none")
	}
}

func TestParseManifest_RawStepMissingPath(t *testing.T) {
	data := []byte(validManifestJSON(`{"type":"raw","method":"POST","file":"raw/a.json"}`))
	_, err := ParseManifest(data)
	if err == nil {
		t.Fatal("expected an error for a raw step with no path, got none")
	}
}

func TestParseManifest_ErrorIncludesManifestID(t *testing.T) {
	// A person debugging a broken seed among several installed ones
	// needs the manifest's own id in the error, not just "step 3 is
	// bad" with no way to tell which seed that was.
	data := []byte(validManifestJSON(`{"type":"fsm"}`))
	_, err := ParseManifest(data)
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); !strings.Contains(got, "test-seed") {
		t.Errorf("error %q does not mention the manifest id", got)
	}
}
