// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package fieldmeta

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ha1tch/xolu/pkg/client"
)

func fakeServer(t *testing.T, handler http.HandlerFunc) *client.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return client.New(server.URL)
}

func TestLoadForEntityType_FiltersToMatchingRows(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": 1, "entity_type": "widgets", "field_name": "flavor", "option_kind": "static",
					"options": []map[string]any{{"key": 1, "value": "Chocolate"}}},
				{"id": 2, "entity_type": "gadgets", "field_name": "status", "option_kind": "static",
					"options": []map[string]any{{"value": "Active"}}},
			},
			"pagination": map[string]any{"page": 1, "per_page": 500, "total_items": 2, "total_pages": 1},
		})
	})

	got, err := LoadForEntityType(context.Background(), c, "widgets")
	if err != nil {
		t.Fatalf("LoadForEntityType: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1 (gadgets row filtered out): %+v", len(got), got)
	}
	m, ok := got["flavor"]
	if !ok {
		t.Fatal(`got["flavor"] missing`)
	}
	if m.EntityType != "widgets" || m.OptionKind != "static" {
		t.Fatalf("m = %+v, unexpected", m)
	}
}

func TestLoadForEntityType_NumericKeyCoercedToString(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": 1, "entity_type": "widgets", "field_name": "flavor", "option_kind": "static",
					"options": []map[string]any{{"key": 1, "value": "Chocolate"}, {"key": 2, "value": "Strawberry"}}},
			},
			"pagination": map[string]any{"page": 1, "per_page": 500, "total_items": 1, "total_pages": 1},
		})
	})

	got, _ := LoadForEntityType(context.Background(), c, "widgets")
	opts := got["flavor"].Options
	if len(opts) != 2 || opts[0].Key != "1" || opts[1].Key != "2" {
		t.Fatalf("opts = %+v, want keys \"1\" and \"2\"", opts)
	}
}

func TestLoadForEntityType_StringKeyDefaultsToValue(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": 1, "entity_type": "widgets", "field_name": "status", "option_kind": "static",
					"options": []map[string]any{{"value": "Visible"}, {"value": "Hidden"}}},
			},
			"pagination": map[string]any{"page": 1, "per_page": 500, "total_items": 1, "total_pages": 1},
		})
	})

	got, _ := LoadForEntityType(context.Background(), c, "widgets")
	opts := got["status"].Options
	if len(opts) != 2 || opts[0].Key != "Visible" || opts[1].Key != "Hidden" {
		t.Fatalf("opts = %+v, want key==value for string options with no explicit key", opts)
	}
}

func TestLoadForEntityType_MissingEntityTypeReturnsEmptyNotError(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-ST001", "message": "not found"}})
	})

	got, err := LoadForEntityType(context.Background(), c, "widgets")
	if err != nil {
		t.Fatalf("LoadForEntityType: %v, want no error when xoluman_field_meta doesn't exist yet", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d entries, want 0", len(got))
	}
}

func TestLoadForEntityType_MalformedRowSkippedNotFatal(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": 1}, // missing entity_type/field_name entirely
				{"id": 2, "entity_type": "widgets", "field_name": "flavor", "option_kind": "static"},
			},
			"pagination": map[string]any{"page": 1, "per_page": 500, "total_items": 2, "total_pages": 1},
		})
	})

	got, err := LoadForEntityType(context.Background(), c, "widgets")
	if err != nil {
		t.Fatalf("LoadForEntityType: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1 (the malformed row skipped, not fatal to the rest): %+v", len(got), got)
	}
}

func TestResolveOptions_StaticReturnsDirectly(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("static options must not make any request")
	})
	m := Meta{OptionKind: "static", Options: []Option{{Key: "1", Value: "Chocolate"}}}

	got, err := ResolveOptions(context.Background(), c, m)
	if err != nil {
		t.Fatalf("ResolveOptions: %v", err)
	}
	if len(got) != 1 || got[0].Value != "Chocolate" {
		t.Fatalf("got = %+v, unexpected", got)
	}
}

func TestResolveOptions_RefFetchesAndParsesTargetDocument(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/system_lists/104" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 104,
			"options": []map[string]any{
				{"key": 1, "value": "Admin"},
				{"key": 2, "value": "Viewer"},
			},
		})
	})
	m := Meta{OptionKind: "ref", RefEntity: "system_lists", RefID: 104, RefOptionsField: "options"}

	got, err := ResolveOptions(context.Background(), c, m)
	if err != nil {
		t.Fatalf("ResolveOptions: %v", err)
	}
	if len(got) != 2 || got[0].Value != "Admin" || got[1].Value != "Viewer" {
		t.Fatalf("got = %+v, unexpected", got)
	}
}

func TestResolveOptions_RefCustomOptionsField(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      104,
			"choices": []map[string]any{{"key": "a", "value": "A"}},
		})
	})
	m := Meta{OptionKind: "ref", RefEntity: "system_lists", RefID: 104, RefOptionsField: "choices"}

	got, err := ResolveOptions(context.Background(), c, m)
	if err != nil {
		t.Fatalf("ResolveOptions: %v", err)
	}
	if len(got) != 1 || got[0].Value != "A" {
		t.Fatalf("got = %+v, unexpected", got)
	}
}

func TestResolveOptions_RefMissingFieldErrors(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 104})
	})
	m := Meta{OptionKind: "ref", RefEntity: "system_lists", RefID: 104, RefOptionsField: "options"}

	_, err := ResolveOptions(context.Background(), c, m)
	if err == nil {
		t.Fatal("want an error when the referenced document has no options field, not a silently empty dropdown")
	}
}

func TestResolveOptions_RefFetchFailureErrors(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-ST001", "message": "not found"}})
	})
	m := Meta{OptionKind: "ref", RefEntity: "system_lists", RefID: 999}

	_, err := ResolveOptions(context.Background(), c, m)
	if err == nil {
		t.Fatal("want an error when the ref target doesn't exist")
	}
}

func TestDecodeMeta_DefaultRefOptionsField(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": 1, "entity_type": "widgets", "field_name": "x", "option_kind": "ref", "ref_entity": "lists", "ref_id": 5},
			},
			"pagination": map[string]any{"page": 1, "per_page": 500, "total_items": 1, "total_pages": 1},
		})
	})
	got, _ := LoadForEntityType(context.Background(), c, "widgets")
	if got["x"].RefOptionsField != "options" {
		t.Fatalf("RefOptionsField = %q, want default %q", got["x"].RefOptionsField, "options")
	}
}

func TestRememberRefTarget_CreatesFieldMetaDoc(t *testing.T) {
	var got map[string]any
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "message": "created"})
	})

	if err := RememberRefTarget(context.Background(), c, "companies", "owner", "users"); err != nil {
		t.Fatalf("RememberRefTarget: %v", err)
	}
	if got["entity_type"] != "companies" || got["field_name"] != "owner" || got["option_kind"] != "ref-target" || got["ref_entity"] != "users" {
		t.Fatalf("created doc = %+v, unexpected", got)
	}
}

func TestLookupRememberedTarget_FindsRefTargetKind(t *testing.T) {
	metas := map[string]Meta{
		"owner": {OptionKind: "ref-target", RefEntity: "users"},
	}
	target, ok := LookupRememberedTarget(metas, "owner")
	if !ok || target != "users" {
		t.Fatalf("got %q, %v, want %q, true", target, ok, "users")
	}
}

func TestLookupRememberedTarget_IgnoresOtherOptionKinds(t *testing.T) {
	// A "ref"-kind Meta's RefEntity means something different (where a
	// select field's options come from) — must not be misread as a
	// remembered ref-field target.
	metas := map[string]Meta{
		"status": {OptionKind: "ref", RefEntity: "status_options", RefID: 1},
	}
	_, ok := LookupRememberedTarget(metas, "status")
	if ok {
		t.Fatal("got ok=true for a \"ref\"-kind Meta, want false — that's a different concept")
	}
}

func TestLookupRememberedTarget_MissingField(t *testing.T) {
	_, ok := LookupRememberedTarget(map[string]Meta{}, "owner")
	if ok {
		t.Fatal("got ok=true for a field with no meta at all, want false")
	}
}
