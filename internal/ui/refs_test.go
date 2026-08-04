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

func TestRefValueInfo_ZeroFloatIsUnset(t *testing.T) {
	if _, _, ok := refValueInfo(float64(0)); ok {
		t.Fatal("refValueInfo(0) ok=true, want false — zero means unset")
	}
}

func TestRefValueInfo_PositiveFloat(t *testing.T) {
	id, label, ok := refValueInfo(float64(42))
	if !ok || id != 42 {
		t.Fatalf("refValueInfo(42.0) = %d, %q, %v, want 42, \"\", true", id, label, ok)
	}
	if label != "" {
		t.Fatalf("label = %q, want empty — a bare number carries no embedded label", label)
	}
}

func TestRefValueInfo_Nil(t *testing.T) {
	if _, _, ok := refValueInfo(nil); ok {
		t.Fatal("refValueInfo(nil) ok=true, want false")
	}
}

func TestRefValueInfo_NumericString(t *testing.T) {
	id, _, ok := refValueInfo("42")
	if !ok || id != 42 {
		t.Fatalf("refValueInfo(\"42\") = %d, %v, want 42, true", id, ok)
	}
}

func TestRefValueInfo_NonNumericString(t *testing.T) {
	if _, _, ok := refValueInfo("not-a-number"); ok {
		t.Fatal("refValueInfo(\"not-a-number\") ok=true, want false")
	}
}

// The tests below are the actual point of this file's existence: xolu
// does not return a bare ID for a ref field on GET — it embeds the
// entire resolved target document ({"id":N,"name":"...",...}), not a
// stub with just an ID. Confirmed directly against a real server, not
// assumed; every ref-handling assumption before this was caught by an
// end-to-end write-then-read round trip was built against a bare-ID
// assumption that never matched what GET actually returns.

func TestRefValueInfo_EmbeddedReadShape_ExtractsIDAndLabel(t *testing.T) {
	id, label, ok := refValueInfo(map[string]any{"id": float64(7), "name": "Alice", "_version": float64(1)})
	if !ok {
		t.Fatal("ok = false, want true for a valid embedded document")
	}
	if id != 7 {
		t.Fatalf("id = %d, want 7", id)
	}
	if label != "Alice" {
		t.Fatalf("label = %q, want %q — the embedded document's own name field, no extra fetch needed", label, "Alice")
	}
}

func TestRefValueInfo_EmbeddedReadShape_TriesLabelFieldsInOrder(t *testing.T) {
	// refLabelFields is ["name", "title", "label"] — confirm "title"
	// is used when "name" isn't present.
	id, label, ok := refValueInfo(map[string]any{"id": float64(3), "title": "Hello World"})
	if !ok || id != 3 {
		t.Fatalf("id, ok = %d, %v, want 3, true", id, ok)
	}
	if label != "Hello World" {
		t.Fatalf("label = %q, want %q", label, "Hello World")
	}
}

func TestRefValueInfo_EmbeddedReadShape_NoRecognisedLabelFieldStillExtractsID(t *testing.T) {
	id, label, ok := refValueInfo(map[string]any{"id": float64(9), "sku": "WX-9"})
	if !ok || id != 9 {
		t.Fatalf("id, ok = %d, %v, want 9, true", id, ok)
	}
	if label != "" {
		t.Fatalf("label = %q, want empty — no recognised label field present", label)
	}
}

func TestRefValueInfo_EmbeddedShapeZeroIDIsUnset(t *testing.T) {
	if _, _, ok := refValueInfo(map[string]any{"id": float64(0), "name": "x"}); ok {
		t.Fatal("ok = true, want false for id 0")
	}
}

func TestRefValueInfo_WriteShape_ExtractsIDNoLabel(t *testing.T) {
	// xolu's own write shape — {"type":"REF","entity":"...","id":N} —
	// carries no label to extract, only an ID.
	id, label, ok := refValueInfo(map[string]any{"type": "REF", "entity": "users", "id": float64(7)})
	if !ok || id != 7 {
		t.Fatalf("id, ok = %d, %v, want 7, true", id, ok)
	}
	if label != "" {
		t.Fatalf("label = %q, want empty — the write shape has no name/title/label field", label)
	}
}

func TestRefValueInfo_MapWithoutIDField(t *testing.T) {
	if _, _, ok := refValueInfo(map[string]any{"name": "Alice"}); ok {
		t.Fatal("ok = true, want false when the map has no id field at all")
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

func TestResolveFormOptions_EmbeddedLabelUsedDirectly_NoExtraFetch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/users/7", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("a separate fetch was made — the embedded document's own label should have been used directly, no round trip needed")
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
	// This is what a real GET actually returns for a ref field — the
	// whole resolved target document, not a bare ID.
	values := formengine.Values{"author_id": map[string]any{"id": float64(7), "name": "Alice"}}

	opts := resolveFormOptions(context.Background(), c, "local", schema, values, nil)

	link, ok := opts.RefLinks["author_id"]
	if !ok {
		t.Fatal("RefLinks[author_id] missing")
	}
	if link.URL != "/connections/local/entities/users/7/edit" {
		t.Fatalf("link.URL = %q, unexpected", link.URL)
	}
	if link.Label != "Alice" {
		t.Fatalf("link.Label = %q, want the embedded document's own name", link.Label)
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
