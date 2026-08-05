// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package formengine

import (
	"net/url"
	"testing"

	"github.com/ha1tch/xolu/pkg/client"
)

func TestParseFormValues_StringField(t *testing.T) {
	fields := []client.FieldDef{{Name: "title", Type: "string"}}
	form := url.Values{"title": {"Hello"}}
	values, errs := ParseFormValues(fields, form, nil)

	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	if values["title"] != "Hello" {
		t.Fatalf("values[title] = %v, want %q", values["title"], "Hello")
	}
}

func TestParseFormValues_OptionalEmptyStringOmitted(t *testing.T) {
	fields := []client.FieldDef{{Name: "nickname", Type: "string"}}
	form := url.Values{"nickname": {""}}
	values, errs := ParseFormValues(fields, form, nil)

	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none for an optional empty field", errs)
	}
	if _, present := values["nickname"]; present {
		t.Fatalf("values[nickname] present = %v, want omitted", values["nickname"])
	}
}

func TestParseFormValues_RequiredEmptyStringErrors(t *testing.T) {
	fields := []client.FieldDef{{Name: "name", Type: "string", Required: true}}
	form := url.Values{"name": {""}}
	_, errs := ParseFormValues(fields, form, nil)

	if errs["name"] == "" {
		t.Fatal("errs[name] empty, want a required-field error")
	}
}

func TestParseFormValues_RequiredWhitespaceOnlyErrors(t *testing.T) {
	fields := []client.FieldDef{{Name: "name", Type: "string", Required: true}}
	form := url.Values{"name": {"   "}}
	_, errs := ParseFormValues(fields, form, nil)

	if errs["name"] == "" {
		t.Fatal("errs[name] empty, want whitespace-only to count as empty for a required field")
	}
}

func TestParseFormValues_AbsentFieldOmitted(t *testing.T) {
	fields := []client.FieldDef{{Name: "title", Type: "string"}}
	values, errs := ParseFormValues(fields, url.Values{}, nil)

	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none for an absent field", errs)
	}
	if _, present := values["title"]; present {
		t.Fatal("values[title] present, want omitted when the field wasn't in the form at all")
	}
}

func TestParseFormValues_IntegerField(t *testing.T) {
	fields := []client.FieldDef{{Name: "age", Type: "integer"}}
	form := url.Values{"age": {"42"}}
	values, errs := ParseFormValues(fields, form, nil)

	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	if values["age"] != float64(42) {
		t.Fatalf("values[age] = %v (%T), want float64(42)", values["age"], values["age"])
	}
}

func TestParseFormValues_IntegerFieldInvalid(t *testing.T) {
	fields := []client.FieldDef{{Name: "age", Type: "integer"}}
	form := url.Values{"age": {"not-a-number"}}
	_, errs := ParseFormValues(fields, form, nil)

	if errs["age"] == "" {
		t.Fatal("errs[age] empty, want a validation error for non-numeric input")
	}
}

func TestParseFormValues_NumberFieldFractional(t *testing.T) {
	fields := []client.FieldDef{{Name: "score", Type: "number"}}
	form := url.Values{"score": {"3.5"}}
	values, _ := ParseFormValues(fields, form, nil)

	if values["score"] != 3.5 {
		t.Fatalf("values[score] = %v, want 3.5", values["score"])
	}
}

func TestParseFormValues_DecimalStaysRawString(t *testing.T) {
	fields := []client.FieldDef{{Name: "price", Type: "string", Format: "decimal"}}
	form := url.Values{"price": {"19.999999999999999999"}}
	values, errs := ParseFormValues(fields, form, nil)

	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	if values["price"] != "19.999999999999999999" {
		t.Fatalf("values[price] = %v, want the exact string, no float64 round-trip", values["price"])
	}
}

func TestParseFormValues_BooleanCheckedWhenPresent(t *testing.T) {
	fields := []client.FieldDef{{Name: "active", Type: "boolean"}}
	form := url.Values{"active": {"on"}}
	values, _ := ParseFormValues(fields, form, nil)

	if values["active"] != true {
		t.Fatalf("values[active] = %v, want true when the field is present", values["active"])
	}
}

func TestParseFormValues_BooleanFalseWhenAbsent(t *testing.T) {
	// The actual browser behaviour this exists to handle: an unchecked
	// checkbox is omitted from form data entirely, not sent as false.
	fields := []client.FieldDef{{Name: "active", Type: "boolean"}}
	values, _ := ParseFormValues(fields, url.Values{}, nil)

	if values["active"] != false {
		t.Fatalf("values[active] = %v, want false when the field is absent (unchecked)", values["active"])
	}
}

func TestParseFormValues_ObjectFieldValidJSON(t *testing.T) {
	fields := []client.FieldDef{{Name: "metadata", Type: "object"}}
	form := url.Values{"metadata": {`{"k":"v"}`}}
	values, errs := ParseFormValues(fields, form, nil)

	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	m, ok := values["metadata"].(map[string]any)
	if !ok || m["k"] != "v" {
		t.Fatalf("values[metadata] = %v, want a parsed map with k=v", values["metadata"])
	}
}

func TestParseFormValues_ObjectFieldInvalidJSON(t *testing.T) {
	fields := []client.FieldDef{{Name: "metadata", Type: "object"}}
	form := url.Values{"metadata": {`{not valid json`}}
	_, errs := ParseFormValues(fields, form, nil)

	if errs["metadata"] == "" {
		t.Fatal("errs[metadata] empty, want a JSON validation error")
	}
}

func TestParseFormValues_ArrayFieldValidJSON(t *testing.T) {
	fields := []client.FieldDef{{Name: "tags", Type: "array"}}
	form := url.Values{"tags": {`["a","b"]`}}
	values, errs := ParseFormValues(fields, form, nil)

	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	arr, ok := values["tags"].([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("values[tags] = %v, want a parsed 2-element slice", values["tags"])
	}
}

func TestParseFormValues_RefFieldNoKnownTarget_NoEntitySupplied_FallsBackToBareNumber(t *testing.T) {
	// No refTargets entry (schema doesn't declare a target) AND no
	// f.Name+"__ref_entity" companion value supplied — the person
	// left the entity-type input blank. Falls back to a bare numeric
	// value, which xolu will reject on write with its own clear
	// validation error; that's the correct outcome for a genuinely
	// unspecified target, not a silent guess. See the next test for
	// the actual fix: when the companion field IS supplied.
	fields := []client.FieldDef{{Name: "author_id", Type: "integer", Format: "ref"}}
	form := url.Values{"author_id": {"7"}}
	values, errs := ParseFormValues(fields, form, nil)

	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	if values["author_id"] != float64(7) {
		t.Fatalf("values[author_id] = %v, want float64(7)", values["author_id"])
	}
}

func TestParseFormValues_RefFieldNoKnownTarget_EntitySupplied_BuildsStructuredWriteShape(t *testing.T) {
	// The actual fix: a real, previously unsolved gap where a ref
	// field whose schema doesn't declare a target (confirmed this is
	// exactly what examples/crm's own seed script does for every ref
	// field except users') could never be written at all — every
	// create or update touching such a field was unconditionally
	// rejected by xolu, regardless of what was typed, since a bare ID
	// is not a valid REF value. refInput now renders a companion
	// f.Name+"__ref_entity" input for exactly this case; this is what
	// consuming it looks like. Verified end-to-end against the real
	// CRM demo through xoluman's actual web form, not just this unit
	// test in isolation — both create and update genuinely succeed
	// now.
	fields := []client.FieldDef{{Name: "author_id", Type: "integer", Format: "ref"}}
	form := url.Values{"author_id": {"7"}, "author_id__ref_entity": {"users"}}
	values, errs := ParseFormValues(fields, form, nil)

	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	got, ok := values["author_id"].(map[string]any)
	if !ok {
		t.Fatalf("values[author_id] = %v (%T), want a structured REF map", values["author_id"], values["author_id"])
	}
	if got["type"] != "REF" || got["entity"] != "users" || got["id"] != int64(7) {
		t.Fatalf("values[author_id] = %+v, want {type:REF, entity:users, id:7}", got)
	}
}

func TestParseFormValues_RefFieldNoKnownTarget_WhitespaceOnlyEntitySupplied_FallsBack(t *testing.T) {
	// A companion field present but blank/whitespace (e.g. the person
	// opened the field and didn't type anything) must not be treated
	// as "entity type: ''" — same honest fallback as no companion
	// field at all, not a structured shape with an empty entity name.
	fields := []client.FieldDef{{Name: "author_id", Type: "integer", Format: "ref"}}
	form := url.Values{"author_id": {"7"}, "author_id__ref_entity": {"   "}}
	values, errs := ParseFormValues(fields, form, nil)

	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	if values["author_id"] != float64(7) {
		t.Fatalf("values[author_id] = %v, want float64(7) (fallback, not a structured shape with an empty entity)", values["author_id"])
	}
}

func TestParseFormValues_RefFieldNoKnownTarget_Invalid(t *testing.T) {
	fields := []client.FieldDef{{Name: "author_id", Type: "integer", Format: "ref"}}
	form := url.Values{"author_id": {"not-an-id"}}
	_, errs := ParseFormValues(fields, form, nil)

	if errs["author_id"] == "" {
		t.Fatal("errs[author_id] empty, want a validation error")
	}
}

func TestParseFormValues_RefFieldKnownTarget_BuildsStructuredWriteShape(t *testing.T) {
	// xolu requires this exact shape for a ref field — confirmed
	// directly against a real server: a bare integer is rejected with
	// XOLU-VL001 ("expected {type,entity,id}, got float64").
	fields := []client.FieldDef{{Name: "author_id", Type: "integer", Format: "ref"}}
	form := url.Values{"author_id": {"7"}}
	values, errs := ParseFormValues(fields, form, map[string]string{"author_id": "users"})

	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	got, ok := values["author_id"].(map[string]any)
	if !ok {
		t.Fatalf("values[author_id] = %v (%T), want a structured REF map", values["author_id"], values["author_id"])
	}
	if got["type"] != "REF" || got["entity"] != "users" || got["id"] != int64(7) {
		t.Fatalf("values[author_id] = %+v, want {type:REF, entity:users, id:7}", got)
	}
}

func TestParseFormValues_RefFieldKnownTarget_Invalid(t *testing.T) {
	fields := []client.FieldDef{{Name: "author_id", Type: "integer", Format: "ref"}}
	form := url.Values{"author_id": {"not-an-id"}}
	_, errs := ParseFormValues(fields, form, map[string]string{"author_id": "users"})

	if errs["author_id"] == "" {
		t.Fatal("errs[author_id] empty, want a validation error")
	}
}

func TestParseFormValues_MultipleFieldsIndependentErrors(t *testing.T) {
	fields := []client.FieldDef{
		{Name: "good", Type: "string"},
		{Name: "bad_number", Type: "integer"},
		{Name: "also_good", Type: "string"},
	}
	form := url.Values{
		"good":       {"fine"},
		"bad_number": {"nope"},
		"also_good":  {"also fine"},
	}
	values, errs := ParseFormValues(fields, form, nil)

	if len(errs) != 1 || errs["bad_number"] == "" {
		t.Fatalf("errs = %v, want exactly one error on bad_number", errs)
	}
	if values["good"] != "fine" || values["also_good"] != "also fine" {
		t.Fatalf("values = %v, want the two valid fields to still parse despite the third's error", values)
	}
}
