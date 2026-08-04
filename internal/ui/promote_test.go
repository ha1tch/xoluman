// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestEntitiesHandler_PromotePreview_RendersSuggestionAndAnalysis(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/entity/gadgets/schema-suggestion", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entity_type":  "gadgets",
			"sampled_rows": 3,
			"total_rows":   3,
			"suggested_schema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"name": map[string]any{"type": "string"}},
			},
			"field_analysis": []map[string]any{
				{"field": "name", "inferred_type": "string", "coverage": 1.0, "confidence": "high"},
				{"field": "status", "inferred_type": "string", "coverage": 0.8, "confidence": "medium", "suggested_enum": []string{"Active", "Inactive"}},
			},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/gadgets/promote", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "gadgets")
	rec := httptest.NewRecorder()

	h.PromotePreview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Sampled 3 of 3", "name", "status", "high", "medium", "Active, Inactive", `id="schema"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
}

func TestEntitiesHandler_Promote_FlexSuccess(t *testing.T) {
	var gotSchema map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/entities/promote/flex/gadgets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotSchema)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "promoted", "auto_inferred": false, "warning": "3 existing rows were not migrated"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	form := url.Values{"schema": {`{"type":"object","properties":{"name":{"type":"string"}}}`}, "mode": {"flex"}}
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/gadgets/promote", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "gadgets")
	rec := httptest.NewRecorder()

	h.Promote(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "promoted") || !strings.Contains(body, "not migrated") {
		t.Fatalf("body missing the success message or warning: %s", body)
	}
	if gotSchema["type"] != "object" {
		t.Fatalf("gotSchema = %+v, the submitted schema wasn't sent through correctly", gotSchema)
	}
}

func TestEntitiesHandler_Promote_StrictSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/entities/promote/strict/gadgets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ticket": "t1", "entity_type": "gadgets", "status": "complete",
			"result": map[string]any{"migrated_rows": 5, "auto_inferred": false},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	form := url.Values{"schema": {`{"type":"object"}`}, "mode": {"strict"}}
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/gadgets/promote", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "gadgets")
	rec := httptest.NewRecorder()

	h.Promote(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "5 row(s) validated and migrated") {
		t.Fatalf("body missing the migration count: %s", rec.Body.String())
	}
}

func TestEntitiesHandler_Promote_StrictRejectedShowsFailuresNotAnError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/entities/promote/strict/gadgets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ticket": "t2", "entity_type": "gadgets", "status": "rejected",
			"failures": []map[string]any{
				{"id": 3, "errors": []string{"missing required field \"name\""}},
			},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	form := url.Values{"schema": {`{"type":"object"}`}, "mode": {"strict"}}
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/gadgets/promote", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "gadgets")
	rec := httptest.NewRecorder()

	h.Promote(rec, req)

	// Rejection is a normal PromoteJobStatus outcome, not a Go error
	// from PromoteStrict — confirmed against pkg/client/schema_promotion.go.
	// The page must render the rejection clearly, not a 502 error page.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d — rejection is a normal result, not an upstream error", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "declined") || !strings.Contains(body, "missing required field") {
		t.Fatalf("body missing the rejection reason: %s", body)
	}
}

func TestEntitiesHandler_Promote_InvalidJSONRedisplaysWithError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/entity/gadgets/schema-suggestion", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entity_type": "gadgets", "sampled_rows": 1, "total_rows": 1,
			"suggested_schema": map[string]any{"type": "object"},
			"field_analysis":   []map[string]any{},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	form := url.Values{"schema": {`{not valid json`}, "mode": {"strict"}}
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/gadgets/promote", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "gadgets")
	rec := httptest.NewRecorder()

	h.Promote(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if !strings.Contains(rec.Body.String(), "Invalid JSON") {
		t.Fatalf("body missing the invalid-JSON message: %s", rec.Body.String())
	}
}

func TestEntitiesHandler_List_SchemalessRowGetsPromoteLink(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/entities", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entities": []map[string]any{
				{"entity_type": "gadgets", "count": float64(1), "has_schema": false},
				{"entity_type": "widgets", "count": float64(1), "has_schema": true},
			},
			"count": 2,
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities", nil)
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.List(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `href="/connections/test/entities/gadgets/promote"`) {
		t.Fatalf("body missing the Promote link for the schemaless row: %s", body)
	}
	if strings.Count(body, "Promote to schema") != 1 {
		t.Fatalf("body has %d Promote links, want exactly 1 (only the schemaless row)", strings.Count(body, "Promote to schema"))
	}
}
