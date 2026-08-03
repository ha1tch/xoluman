// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEntitiesHandler_GridData_ShapeMatchesTabulator(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/widgets/grid-data", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.GridData(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var resp gridDataResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not the expected shape: %v; body = %s", err, rec.Body.String())
	}
	if len(resp.Data) != 2 {
		t.Fatalf("got %d rows, want 2 (fakeXoluWithWidgets seeds 2)", len(resp.Data))
	}
	if resp.Data[0]["id"] == nil {
		t.Fatal("row missing id — the grid's save step needs this to know which row it's editing")
	}
	if resp.Data[0]["name"] != "First widget" {
		t.Fatalf("row[0][name] = %v, want %q", resp.Data[0]["name"], "First widget")
	}
}

func TestEntitiesHandler_GridData_PaginationParamsRespected(t *testing.T) {
	var gotQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/widgets", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       []map[string]any{},
			"pagination": map[string]any{"page": 3, "per_page": 10, "total_items": 25, "total_pages": 3},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/widgets/grid-data?page=3&per_page=10", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.GridData(rec, req)

	if gotQuery != "page=3&per_page=10" {
		t.Fatalf("upstream query = %q, want page=3&per_page=10", gotQuery)
	}

	var resp gridDataResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.LastPage != 3 || resp.LastRow != 25 {
		t.Fatalf("resp = %+v, want LastPage=3 LastRow=25", resp)
	}
}

func TestEntitiesHandler_GridData_UnknownConnection(t *testing.T) {
	h := NewEntitiesHandler(newTestStore(t))
	req := httptest.NewRequest(http.MethodGet, "/connections/nope/entities/widgets/grid-data", nil)
	req.SetPathValue("name", "nope")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.GridData(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json even for the error path", ct)
	}
}

func TestEntitiesHandler_GridSave_PatchesEachRowIndependently(t *testing.T) {
	var patched []map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v1/widgets/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		patched = append(patched, body)
		if body["count"] == float64(999) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-VL001", "message": "count too large"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "updated"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	body, _ := json.Marshal([]gridSaveRequest{
		{ID: 1, Changes: map[string]any{"count": float64(5)}},
		{ID: 2, Changes: map[string]any{"count": float64(999)}}, // deliberately fails
		{ID: 3, Changes: map[string]any{"name": "Renamed"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/widgets/grid-data", bytes.NewReader(body))
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.GridSave(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(patched) != 3 {
		t.Fatalf("upstream received %d PATCH requests, want 3 (one per row attempted regardless of earlier failures)", len(patched))
	}

	var results []gridSaveResult
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("response not the expected shape: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	if !results[0].Success || !results[2].Success {
		t.Fatalf("results[0]/[2] should have succeeded: %+v", results)
	}
	if results[1].Success {
		t.Fatal("results[1] should have failed (deliberately)")
	}
	if results[1].Message == "" {
		t.Fatal("a failed row's Message is empty, want the upstream error text")
	}

	// The critical design guarantee from T-11's design pass: only the
	// changed field is ever sent, never a full document.
	for _, p := range patched {
		if len(p) != 1 {
			t.Fatalf("a PATCH body had %d fields, want exactly 1 (the single changed field) — sending more risks looking like Update semantics: %v", len(p), p)
		}
	}
}

func TestEntitiesHandler_GridSave_EmptyBatch(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/widgets/grid-data", bytes.NewReader([]byte("[]")))
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.GridSave(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var results []gridSaveResult
	_ = json.Unmarshal(rec.Body.Bytes(), &results)
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0 for an empty batch", len(results))
	}
}

func TestEntitiesHandler_GridSave_MalformedBody(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/widgets/grid-data", bytes.NewReader([]byte("not json")))
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.GridSave(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
