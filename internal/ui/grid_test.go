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

func TestEntitiesHandler_GridSave_ReconstructsRefFieldFromRawID(t *testing.T) {
	var patchedBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/deals", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// A real JSON Schema document -- client.GetEntitySchema's own
		// extractFieldsFromSchema parses this shape (properties +
		// format:ref + the xolu-specific target extension), not a
		// flat {refs:[...], fields:[...]} envelope.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"owner": map[string]any{"type": "object", "format": "ref", "target": "users"},
				"name":  map[string]any{"type": "string"},
			},
		})
	})
	mux.HandleFunc("PATCH /api/v1/deals/{id}", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&patchedBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "updated"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	// Exactly what Tabulator's plain "input" editor sends for a REF
	// column today: the raw target ID the person typed, nothing else —
	// confirmed directly against a real grid save attempt before this
	// fix, which xolu correctly rejected outright (XOLU-VL001).
	body, _ := json.Marshal([]gridSaveRequest{
		{ID: 1, Changes: map[string]any{"owner": float64(7)}},
	})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/deals/grid-data", bytes.NewReader(body))
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "deals")
	rec := httptest.NewRecorder()

	h.GridSave(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	owner, ok := patchedBody["owner"].(map[string]any)
	if !ok {
		t.Fatalf("patched owner field = %+v (%T), want a structured REF object", patchedBody["owner"], patchedBody["owner"])
	}
	if owner["type"] != "REF" || owner["entity"] != "users" || owner["id"] != float64(7) {
		t.Errorf("REF value = %+v, want type=REF entity=users id=7", owner)
	}
}

func TestEntitiesHandler_GridSave_NonRefFieldsPassThroughUnchanged(t *testing.T) {
	var patchedBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/deals", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"owner": map[string]any{"type": "object", "format": "ref", "target": "users"},
			},
		})
	})
	mux.HandleFunc("PATCH /api/v1/deals/{id}", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&patchedBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "updated"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	body, _ := json.Marshal([]gridSaveRequest{
		{ID: 1, Changes: map[string]any{"name": "Renamed Deal"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/deals/grid-data", bytes.NewReader(body))
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "deals")
	rec := httptest.NewRecorder()

	h.GridSave(rec, req)

	if patchedBody["name"] != "Renamed Deal" {
		t.Errorf("name field = %v, want it passed through completely unchanged", patchedBody["name"])
	}
	if _, present := patchedBody["owner"]; present {
		t.Error("owner should not appear at all — it was never part of this row's changes")
	}
}

func TestEntitiesHandler_GridSave_InvalidRefValueFailsThatRowOnly(t *testing.T) {
	var patchCount int
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/deals", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"owner": map[string]any{"type": "object", "format": "ref", "target": "users"},
			},
		})
	})
	mux.HandleFunc("PATCH /api/v1/deals/{id}", func(w http.ResponseWriter, r *http.Request) {
		patchCount++
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "updated"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	body, _ := json.Marshal([]gridSaveRequest{
		{ID: 1, Changes: map[string]any{"owner": "not-a-number"}},
		{ID: 2, Changes: map[string]any{"name": "Still Fine"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/deals/grid-data", bytes.NewReader(body))
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "deals")
	rec := httptest.NewRecorder()

	h.GridSave(rec, req)

	var results []gridSaveResult
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("response not the expected shape: %v", err)
	}
	if results[0].Success {
		t.Error("row 1 (invalid REF value) should have failed client-side, before ever reaching PATCH")
	}
	if !results[1].Success {
		t.Error("row 2 should still succeed independently of row 1's failure")
	}
	if patchCount != 1 {
		t.Errorf("PATCH called %d times, want exactly 1 — row 1's bad value should never have reached the network", patchCount)
	}
}

func TestReconstructRefValues_NoRefFieldsPassesEverythingThrough(t *testing.T) {
	changes := map[string]any{"owner": float64(7), "name": "x"}
	out, learned, err := reconstructRefValues(changes, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["owner"] != float64(7) {
		t.Errorf("owner = %v, want passed through unchanged when the field isn't a known REF field", out["owner"])
	}
	if learned != nil {
		t.Errorf("learned = %+v, want nil — nothing was ever a REF field to learn a target for", learned)
	}
}

func TestReconstructRefValues_CompoundFormWhenTargetUnknown(t *testing.T) {
	changes := map[string]any{"owner": "users:5"}
	refFieldNames := map[string]bool{"owner": true}

	out, learned, err := reconstructRefValues(changes, map[string]string{}, refFieldNames)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	owner, ok := out["owner"].(map[string]any)
	if !ok || owner["type"] != "REF" || owner["entity"] != "users" || owner["id"] != int64(5) {
		t.Errorf("owner = %+v, want a REF object built from the compound form", out["owner"])
	}
	if learned["owner"] != "users" {
		t.Errorf("learned = %+v, want owner -> users so it's remembered for next time", learned)
	}
}

func TestReconstructRefValues_PlainIDRejectedWhenTargetUnknown(t *testing.T) {
	changes := map[string]any{"owner": float64(5)}
	refFieldNames := map[string]bool{"owner": true}

	_, _, err := reconstructRefValues(changes, map[string]string{}, refFieldNames)
	if err == nil {
		t.Fatal("expected an error for a plain ID with no known target and no compound form, got nil")
	}
}

func TestReconstructRefValues_KnownTargetDoesNotNeedCompoundForm(t *testing.T) {
	changes := map[string]any{"owner": float64(5)}
	refFieldNames := map[string]bool{"owner": true}

	out, learned, err := reconstructRefValues(changes, map[string]string{"owner": "users"}, refFieldNames)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	owner, ok := out["owner"].(map[string]any)
	if !ok || owner["entity"] != "users" || owner["id"] != int64(5) {
		t.Errorf("owner = %+v, want a REF object using the already-known target", out["owner"])
	}
	if learned != nil {
		t.Errorf("learned = %+v, want nil — nothing new was learned, the target was already known", learned)
	}
}

func TestCoerceToInt64_AcceptsBothNumberAndStringForms(t *testing.T) {
	if v, err := coerceToInt64(float64(42)); err != nil || v != 42 {
		t.Errorf("float64(42): got (%d, %v), want (42, nil)", v, err)
	}
	if v, err := coerceToInt64("42"); err != nil || v != 42 {
		t.Errorf(`"42": got (%d, %v), want (42, nil)`, v, err)
	}
	if v, err := coerceToInt64(" 42 "); err != nil || v != 42 {
		t.Errorf(`" 42 ": got (%d, %v), want (42, nil) -- Tabulator's own input editor can leave stray whitespace`, v, err)
	}
	if _, err := coerceToInt64("not-a-number"); err == nil {
		t.Error(`"not-a-number": expected an error, got nil`)
	}
	if _, err := coerceToInt64(true); err == nil {
		t.Error("bool: expected an error for an unsupported type, got nil")
	}
}

func TestEntitiesHandler_GridSave_CompoundFormWhenSchemaHasNoTarget(t *testing.T) {
	var patchedBody map[string]any
	var rememberedTarget string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/deals", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// The real, confirmed-common case: a REF field with no
		// "target" extension at all (this session's own CRM example
		// schema does exactly this for every REF field except one) —
		// refTargetsByField alone leaves this field's grid column
		// unusable; this is what the compound "entityType:id" form
		// exists for.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"owner": map[string]any{"type": "object", "format": "ref"},
			},
		})
	})
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "pagination": map[string]any{"total_pages": 1}})
	})
	mux.HandleFunc("POST /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		rememberedTarget, _ = body["ref_entity"].(string)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
	})
	mux.HandleFunc("PATCH /api/v1/deals/{id}", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&patchedBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "updated"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	body, _ := json.Marshal([]gridSaveRequest{
		{ID: 1, Changes: map[string]any{"owner": "users:5"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/deals/grid-data", bytes.NewReader(body))
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "deals")
	rec := httptest.NewRecorder()

	h.GridSave(rec, req)

	var results []gridSaveResult
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("response not the expected shape: %v", err)
	}
	if !results[0].Success {
		t.Fatalf("expected success, got: %+v", results[0])
	}
	owner, ok := patchedBody["owner"].(map[string]any)
	if !ok || owner["entity"] != "users" || owner["id"] != float64(5) {
		t.Errorf("patched owner = %+v, want a REF object built from the compound form", patchedBody["owner"])
	}
	if rememberedTarget != "users" {
		t.Errorf("remembered target = %q, want \"users\" -- the compound form should teach xoluman the target for next time", rememberedTarget)
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
