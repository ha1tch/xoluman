// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/formengine"
)

func TestRefTargetID_ZeroFloatIsUnset(t *testing.T) {
	if _, ok := refTargetID(float64(0)); ok {
		t.Fatal("refTargetID(0) ok=true, want false — zero means unset")
	}
}

func TestRefTargetID_PositiveFloat(t *testing.T) {
	id, ok := refTargetID(float64(42))
	if !ok || id != 42 {
		t.Fatalf("refTargetID(42.0) = %d, %v, want 42, true", id, ok)
	}
}

func TestRefTargetID_Nil(t *testing.T) {
	if _, ok := refTargetID(nil); ok {
		t.Fatal("refTargetID(nil) ok=true, want false")
	}
}

func TestRefTargetID_NumericString(t *testing.T) {
	id, ok := refTargetID("42")
	if !ok || id != 42 {
		t.Fatalf("refTargetID(\"42\") = %d, %v, want 42, true", id, ok)
	}
}

func TestRefTargetID_NonNumericString(t *testing.T) {
	if _, ok := refTargetID("not-a-number"); ok {
		t.Fatal("refTargetID(\"not-a-number\") ok=true, want false")
	}
}

func TestResolveRefLabel_UsesNameField(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/users/7", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 7, "name": "Alice"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := xclient.New(server.URL)

	label := resolveRefLabel(context.Background(), c, "users", 7)
	if label != "Alice" {
		t.Fatalf("label = %q, want %q", label, "Alice")
	}
}

func TestResolveRefLabel_FallsBackToTitleThenLabel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/posts/3", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 3, "title": "Hello World"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := xclient.New(server.URL)

	label := resolveRefLabel(context.Background(), c, "posts", 3)
	if label != "Hello World" {
		t.Fatalf("label = %q, want %q", label, "Hello World")
	}
}

func TestResolveRefLabel_NoRecognisedFieldFallsBackToID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/widgets/9", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 9, "sku": "WX-9"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := xclient.New(server.URL)

	label := resolveRefLabel(context.Background(), c, "widgets", 9)
	if label != "widgets #9" {
		t.Fatalf("label = %q, want %q", label, "widgets #9")
	}
}

func TestResolveRefLabel_FetchFailureFallsBackToID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/widgets/999", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-ST001", "message": "not found"}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := xclient.New(server.URL)

	label := resolveRefLabel(context.Background(), c, "widgets", 999)
	if label != "widgets #999" {
		t.Fatalf("label = %q, want a fallback label even when the fetch fails, not a crash", label)
	}
}

func TestResolveFormOptions_BuildsRefLinkForSetRefField(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/users/7", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 7, "name": "Alice"})
	})
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-ST001", "message": "not found"}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := xclient.New(server.URL)

	schema := &xclient.EntitySchema{
		Name:   "posts",
		Fields: []xclient.FieldDef{{Name: "author_id", Type: "integer", Format: "ref"}},
		Refs:   []xclient.RefFieldDef{{Name: "author_id", Target: "users"}},
	}
	values := formengine.Values{"author_id": float64(7)}

	opts := resolveFormOptions(context.Background(), c, "local", schema, values, nil)

	link, ok := opts.RefLinks["author_id"]
	if !ok {
		t.Fatal("RefLinks[author_id] missing")
	}
	if link.URL != "/connections/local/entities/users/7/edit" {
		t.Fatalf("link.URL = %q, unexpected", link.URL)
	}
	if link.Label != "Alice" {
		t.Fatalf("link.Label = %q, want the resolved name", link.Label)
	}
}

func TestResolveFormOptions_NoRefLinkWhenFieldUnset(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-ST001", "message": "not found"}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := xclient.New(server.URL)

	schema := &xclient.EntitySchema{
		Name: "posts",
		Refs: []xclient.RefFieldDef{{Name: "author_id", Target: "users"}},
	}

	opts := resolveFormOptions(context.Background(), c, "local", schema, formengine.Values{}, nil)

	if _, ok := opts.RefLinks["author_id"]; ok {
		t.Fatal("RefLinks[author_id] present, want none for an unset ref field")
	}
}

func TestResolveFormOptions_SkipsPolymorphicRef(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-ST001", "message": "not found"}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := xclient.New(server.URL)

	schema := &xclient.EntitySchema{
		Name: "posts",
		Refs: []xclient.RefFieldDef{{Name: "target_id", Target: ""}}, // polymorphic — no single target type
	}
	values := formengine.Values{"target_id": float64(5)}

	opts := resolveFormOptions(context.Background(), c, "local", schema, values, nil)

	if _, ok := opts.RefLinks["target_id"]; ok {
		t.Fatal("RefLinks[target_id] present, want none for a polymorphic ref with no single target type")
	}
}

func TestResolveFormOptions_FieldOptionsPopulatedFromFieldMeta(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": 1, "entity_type": "widgets", "field_name": "flavor", "option_kind": "static",
					"options": []map[string]any{{"key": 1, "value": "Chocolate"}}},
			},
			"pagination": map[string]any{"page": 1, "per_page": 500, "total_items": 1, "total_pages": 1},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := xclient.New(server.URL)

	schema := &xclient.EntitySchema{Name: "widgets"}
	opts := resolveFormOptions(context.Background(), c, "local", schema, formengine.Values{}, nil)

	got := opts.FieldOptions["flavor"]
	if len(got) != 1 || got[0].Key != "1" || got[0].Value != "Chocolate" {
		t.Fatalf("FieldOptions[flavor] = %+v, unexpected", got)
	}
}
