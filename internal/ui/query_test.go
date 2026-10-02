// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestQueryHandler_ListSavedQueries_EmptyWhenEntityTypeNeverCreated(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/xoluman_saved_query", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-ST008", "message": "not found"}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/query/saved?mode=oql", nil)
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()
	h.ListSavedQueries(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []savedQuery
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d saved queries, want 0", len(got))
	}
}

func TestQueryHandler_ListSavedQueries_FiltersByModeAndOrdersNewestFirst(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/xoluman_saved_query", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": float64(1), "mode": "oql", "name": "Older OQL", "query": "SELECT 1", "created_at": "2026-08-01T00:00:00Z"},
				{"id": float64(2), "mode": "sulpher", "name": "A Cypher one", "query": "MATCH (n) RETURN n", "created_at": "2026-08-02T00:00:00Z"},
				{"id": float64(3), "mode": "oql", "name": "Newer OQL", "query": "SELECT 2", "created_at": "2026-08-03T00:00:00Z"},
			},
			"pagination": map[string]any{"page": 1, "per_page": 1000, "total_items": 3, "total_pages": 1},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/query/saved?mode=oql", nil)
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()
	h.ListSavedQueries(rec, req)

	var got []savedQuery
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d queries, want 2 (sulpher one filtered out): %+v", len(got), got)
	}
	if got[0].Name != "Newer OQL" || got[1].Name != "Older OQL" {
		t.Fatalf("order = [%q, %q], want newest first", got[0].Name, got[1].Name)
	}
}

func TestQueryHandler_ListSavedQueries_InvalidMode(t *testing.T) {
	h := &queryHandler{store: newTestStore(t)}
	req := httptest.NewRequest(http.MethodGet, "/connections/test/query/saved?mode=nonsense", nil)
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()
	h.ListSavedQueries(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestQueryHandler_CreateSavedQuery_OQL(t *testing.T) {
	var got map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/xoluman_saved_query", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(7), "message": "created"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	body, _ := json.Marshal(savedQuery{Mode: "oql", Name: "My query", Query: "SELECT * FROM widgets"})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/saved", bytes.NewReader(body))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()
	h.CreateSavedQuery(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got["mode"] != "oql" || got["name"] != "My query" || got["query"] != "SELECT * FROM widgets" {
		t.Fatalf("upstream create body = %+v, unexpected", got)
	}
	if _, present := got["created_at"]; !present {
		t.Fatalf("upstream create body missing created_at: %+v", got)
	}
}

func TestQueryHandler_CreateSavedQuery_RESTStoresAllFourFields(t *testing.T) {
	var got map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/xoluman_saved_query", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(8), "message": "created"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	body, _ := json.Marshal(savedQuery{Mode: "rest", Name: "List entities", Method: "GET", Path: "/api/v1/entities", ContentType: "application/json", Body: ""})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/saved", bytes.NewReader(body))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()
	h.CreateSavedQuery(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got["method"] != "GET" || got["path"] != "/api/v1/entities" {
		t.Fatalf("upstream create body = %+v, missing REST-specific fields", got)
	}
	if _, present := got["query"]; present {
		t.Fatalf("upstream create body = %+v, want no query field for REST mode", got)
	}
}

func TestQueryHandler_CreateSavedQuery_MissingNameRejected(t *testing.T) {
	h := &queryHandler{store: seedConnection(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).URL)}
	body, _ := json.Marshal(savedQuery{Mode: "oql", Name: "  ", Query: "SELECT 1"})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/saved", bytes.NewReader(body))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()
	h.CreateSavedQuery(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestQueryHandler_SaveGraphNode_PatchesTheRightEntity(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	body, _ := json.Marshal(graphNodeSaveRequest{
		Type: "deals", ID: 6,
		Changes: map[string]any{"name": "Renamed via graph viewer"},
	})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph/node", bytes.NewReader(body))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.SaveGraphNode(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if gotMethod != http.MethodPatch {
		t.Fatalf("upstream method = %q, want PATCH", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/deals/6") {
		t.Fatalf("upstream path = %q, want it to target deals/6", gotPath)
	}
	if !strings.Contains(string(gotBody), "Renamed via graph viewer") {
		t.Fatalf("upstream body = %s, want the changed field", gotBody)
	}
}

func TestQueryHandler_SaveGraphNode_MissingTypeOrID(t *testing.T) {
	store := seedConnection(t, "http://unused.invalid")
	h := &queryHandler{store: store}

	body, _ := json.Marshal(graphNodeSaveRequest{Changes: map[string]any{"name": "x"}})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph/node", bytes.NewReader(body))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.SaveGraphNode(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d for a missing type/id", rec.Code, http.StatusBadRequest)
	}
}

func TestQueryHandler_SaveGraphEdge_RetargetsViaPatchOnSourceEntity(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	body, _ := json.Marshal(graphEdgeSaveRequest{
		From: "deals:6", RelField: "primary_contact", NewTo: "contacts:22",
	})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph/edge", bytes.NewReader(body))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.SaveGraphEdge(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if !strings.HasSuffix(gotPath, "/deals/6") {
		t.Fatalf("upstream path = %q, want it to patch the SOURCE entity deals/6, not the target", gotPath)
	}
	ref, ok := gotBody["primary_contact"].(map[string]any)
	if !ok {
		t.Fatalf("patched body = %+v, want a primary_contact REF object", gotBody)
	}
	if ref["entity"] != "contacts" || ref["id"] != float64(22) || ref["type"] != "REF" {
		t.Fatalf("REF value = %+v, want entity=contacts id=22 type=REF (the new target, not the old one)", ref)
	}
}

func TestQueryHandler_SaveGraphEdge_EmptyNewToClearsTheRelationship(t *testing.T) {
	var gotPath string
	var gotRaw string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotRaw = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	body, _ := json.Marshal(graphEdgeSaveRequest{
		From: "tasks:1", RelField: "contact", NewTo: "",
	})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph/edge", bytes.NewReader(body))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.SaveGraphEdge(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if !strings.HasSuffix(gotPath, "/tasks/1") {
		t.Fatalf("upstream path = %q, want it to patch tasks/1", gotPath)
	}
	var gotBody map[string]any
	if err := json.Unmarshal([]byte(gotRaw), &gotBody); err != nil {
		t.Fatalf("patched body not valid JSON: %v (%s)", err, gotRaw)
	}
	val, present := gotBody["contact"]
	if !present {
		t.Fatalf("patched body = %s, want a \"contact\" key present with a null value", gotRaw)
	}
	if val != nil {
		t.Fatalf("contact value = %v, want null (clearing the field, not setting some other value)", val)
	}
}

func TestQueryHandler_SaveGraphEdge_MalformedReferenceIsRejected(t *testing.T) {
	store := seedConnection(t, "http://unused.invalid")
	h := &queryHandler{store: store}

	body, _ := json.Marshal(graphEdgeSaveRequest{
		From: "not-a-valid-reference", RelField: "primary_contact", NewTo: "contacts:22",
	})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph/edge", bytes.NewReader(body))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.SaveGraphEdge(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d for a malformed \"from\" reference", rec.Code, http.StatusBadRequest)
	}
}

func TestParseGraphNodeID(t *testing.T) {
	cases := []struct {
		in       string
		wantType string
		wantID   int64
		wantErr  bool
	}{
		{"deals:6", "deals", 6, false},
		{"contacts:123", "contacts", 123, false},
		{"no-colon-here", "", 0, true},
		{"deals:not-a-number", "", 0, true},
		{"", "", 0, true},
	}
	for _, tc := range cases {
		gotType, gotID, err := parseGraphNodeID(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseGraphNodeID(%q): got nil error, want one", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseGraphNodeID(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if gotType != tc.wantType || gotID != tc.wantID {
			t.Errorf("parseGraphNodeID(%q) = (%q, %d), want (%q, %d)", tc.in, gotType, gotID, tc.wantType, tc.wantID)
		}
	}
}

func TestQueryHandler_GraphSulpherDormant_ClassifiesNodesAndEdges(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/graph/query", func(w http.ResponseWriter, r *http.Request) {
		// The real shape confirmed directly against a running xolu
		// instance, not assumed: a node has "_id" and "type"; an edge
		// has "from"/"rel"/"to" and no "_id" at all.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "completed",
			"result": []map[string]any{
				{
					"d": map[string]any{"_id": "1", "id": float64(1), "type": "deals", "name": "Big Deal"},
					"r": map[string]any{"from": "deals:1", "rel": "primary_contact", "to": "contacts:17"},
					"p": map[string]any{"_id": "17", "id": float64(17), "type": "contacts", "first_name": "Henry"},
				},
			},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(queryRunRequest{Query: "MATCH (d:deals)-[r:primary_contact]->(p:contacts) RETURN d, r, p"})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.graphSulpherDormant(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got graphData
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2 (one deal, one contact): %+v", len(got.Nodes), got.Nodes)
	}
	if len(got.Edges) != 1 {
		t.Fatalf("got %d edges, want 1: %+v", len(got.Edges), got.Edges)
	}
	if got.Edges[0].From != "deals:1" || got.Edges[0].To != "contacts:17" || got.Edges[0].Rel != "primary_contact" {
		t.Fatalf("edge = %+v, unexpected", got.Edges[0])
	}
}

func TestQueryHandler_GraphSulpherDormant_DeduplicatesNodesAndEdgesAcrossRows(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/graph/query", func(w http.ResponseWriter, r *http.Request) {
		row := map[string]any{
			"d": map[string]any{"_id": "1", "id": float64(1), "type": "deals", "name": "Big Deal"},
			"r": map[string]any{"from": "deals:1", "rel": "primary_contact", "to": "contacts:17"},
			"p": map[string]any{"_id": "17", "id": float64(17), "type": "contacts", "first_name": "Henry"},
		}
		// Same deal, same edge, same contact -- appears identically in
		// two rows, matching what a real multi-hop query naturally
		// produces when two separate paths pass through the same node.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "completed",
			"result": []map[string]any{row, row},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(queryRunRequest{Query: "MATCH (d:deals)-[r:primary_contact]->(p:contacts) RETURN d, r, p"})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.graphSulpherDormant(rec, req)

	var got graphData
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("got %d nodes across 2 identical rows, want 2 deduplicated: %+v", len(got.Nodes), got.Nodes)
	}
	if len(got.Edges) != 1 {
		t.Fatalf("got %d edges across 2 identical rows, want 1 deduplicated: %+v", len(got.Edges), got.Edges)
	}
}

func TestQueryHandler_GraphSulpherDormant_SkipsScalarValues(t *testing.T) {
	// A query mixing an aggregate (e.g. COUNT(*)) with a node variable
	// is valid Sulpher -- the scalar just isn't drawable, and must not
	// crash the classification pass.
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/graph/query", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "completed",
			"result": []map[string]any{
				{
					"d":     map[string]any{"_id": "1", "id": float64(1), "type": "deals", "name": "Big Deal"},
					"total": float64(42),
				},
			},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(queryRunRequest{Query: "MATCH (d:deals) RETURN d, COUNT(*) AS total"})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.graphSulpherDormant(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got graphData
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if len(got.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1 (the scalar 'total' should be skipped, not crash or become a node)", len(got.Nodes))
	}
}

func TestQueryHandler_Graph_EmptyResultReturnsEmptyArraysNotNull(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/deals", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "object", "properties": map[string]any{}})
	})
	mux.HandleFunc("GET /api/v1/deals", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{}, "pagination": map[string]any{"page": 1, "per_page": 10, "total_items": 0, "total_pages": 1}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(graphRunRequest{EntityType: "deals", Depth: 1})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.Graph(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "null") {
		t.Fatalf("body = %q, want empty arrays not null (the JS side does .map/.forEach on these directly)", body)
	}
}

func TestQueryHandler_Graph_DelegatesToRestEmbedGraphRunner(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/deals", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":       "object",
			"properties": map[string]any{"owner": map[string]any{"type": "object", "format": "ref"}},
		})
	})
	mux.HandleFunc("GET /api/v1/schema/users", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "object", "properties": map[string]any{}})
	})
	mux.HandleFunc("GET /api/v1/deals", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       []map[string]any{{"id": float64(1), "name": "Big Deal", "owner": map[string]any{"type": "REF", "entity": "users", "id": float64(4)}}},
			"pagination": map[string]any{"page": 1, "per_page": 10, "total_items": 1, "total_pages": 1},
		})
	})
	mux.HandleFunc("GET /api/v1/users/4", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(4), "name": "Alice"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(graphRunRequest{EntityType: "deals", Depth: 1})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.Graph(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got graphData
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if len(got.Nodes) != 2 || len(got.Edges) != 1 {
		t.Fatalf("got %+v, want 2 nodes (deals:1, users:4) and 1 edge", got)
	}
}

func TestQueryHandler_Graph_MissingEntityTypeRejected(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(graphRunRequest{})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.Graph(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("status = %d, want a non-200 for a missing entity type; body = %s", rec.Code, rec.Body.String())
	}
}

func TestQueryHandler_Graph_MalformedBodyRejected(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph", bytes.NewReader([]byte("not json")))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.Graph(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestQueryHandler_Graph_UnknownConnection(t *testing.T) {
	store := seedConnection(t, "http://unused.invalid")
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(graphRunRequest{EntityType: "deals"})
	req := httptest.NewRequest(http.MethodPost, "/connections/does-not-exist/query/graph", bytes.NewReader(reqBody))
	req.SetPathValue("name", "does-not-exist")
	rec := httptest.NewRecorder()

	h.Graph(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestQueryHandler_GraphEntityTypes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/entities", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entities": []map[string]any{
				{"entity_type": "deals", "count": float64(25), "has_schema": true},
				{"entity_type": "companies", "count": float64(12), "has_schema": true},
			},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/query/graph/entity-types", nil)
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.GraphEntityTypes(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON array: %v", err)
	}
	if len(got) != 2 || got[0] != "deals" || got[1] != "companies" {
		t.Fatalf("got %+v, want [deals companies]", got)
	}
}

func TestQueryHandler_ExpandGraphNode(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/users", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "object", "properties": map[string]any{}})
	})
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "pagination": map[string]any{"total_pages": 1}})
	})
	mux.HandleFunc("GET /api/v1/users/7", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(7), "name": "Alice"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(graphExpandRequest{Type: "users", ID: 7, Depth: 1})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph/expand", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.ExpandGraphNode(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got graphData
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	node, ok := nodeByID(got, "users:7")
	if !ok || node.Collapsed || node.Data["name"] != "Alice" {
		t.Fatalf("got %+v, want users:7 present, not collapsed, with real data", got)
	}
}

func TestQueryHandler_ExpandGraphNode_DefaultsDepthWhenUnset(t *testing.T) {
	var gotUserFetch bool
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/users", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":       "object",
			"properties": map[string]any{"manager": map[string]any{"type": "object", "format": "ref"}},
		})
	})
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "pagination": map[string]any{"total_pages": 1}})
	})
	mux.HandleFunc("GET /api/v1/users/7", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(7), "manager": map[string]any{"type": "REF", "entity": "users", "id": float64(1)}})
	})
	mux.HandleFunc("GET /api/v1/users/1", func(w http.ResponseWriter, r *http.Request) {
		gotUserFetch = true
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(1), "name": "Boss"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	// Depth omitted entirely (zero value) -- should still default to a
	// real depth (1), not "expand nothing beyond the seed itself."
	reqBody, _ := json.Marshal(graphExpandRequest{Type: "users", ID: 7})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph/expand", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.ExpandGraphNode(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !gotUserFetch {
		t.Error("users:1 was never fetched -- depth should have defaulted to at least 1, not 0")
	}
}

func TestQueryHandler_ExpandGraphNode_MalformedBodyRejected(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph/expand", bytes.NewReader([]byte("not json")))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.ExpandGraphNode(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestQueryHandler_ExpandGraphNode_UnknownConnection(t *testing.T) {
	store := seedConnection(t, "http://unused.invalid")
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(graphExpandRequest{Type: "users", ID: 7})
	req := httptest.NewRequest(http.MethodPost, "/connections/does-not-exist/query/graph/expand", bytes.NewReader(reqBody))
	req.SetPathValue("name", "does-not-exist")
	rec := httptest.NewRecorder()

	h.ExpandGraphNode(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestQueryHandler_DeleteSavedQuery(t *testing.T) {
	var gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/xoluman_saved_query/7", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	req := httptest.NewRequest(http.MethodDelete, "/connections/test/query/saved/7", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("id", "7")
	rec := httptest.NewRecorder()
	h.DeleteSavedQuery(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if gotPath != "/api/v1/xoluman_saved_query/7" {
		t.Fatalf("upstream delete path = %q, unexpected", gotPath)
	}
}

func TestQueryHandler_DeleteSavedQuery_InvalidID(t *testing.T) {
	h := &queryHandler{store: newTestStore(t)}
	req := httptest.NewRequest(http.MethodDelete, "/connections/test/query/saved/notanumber", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("id", "notanumber")
	rec := httptest.NewRecorder()
	h.DeleteSavedQuery(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestQueryHandler_View_RendersEditorElement(t *testing.T) {
	store := seedConnection(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).URL)
	h := &queryHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/query", nil)
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.View(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<xolu-query-editor") {
		t.Fatalf("body missing the query editor custom element: %s", body)
	}
	if !strings.Contains(body, `run-url="/connections/test/query/run"`) {
		t.Fatalf("body missing the correct run-url attribute: %s", body)
	}
	if !strings.Contains(body, "/static/vendor/codemirror-bundle@1.js") {
		t.Fatalf("body missing the CodeMirror bundle import map entry: %s", body)
	}
}

func TestQueryHandler_View_UnknownConnection(t *testing.T) {
	h := &queryHandler{store: newTestStore(t)}
	req := httptest.NewRequest(http.MethodGet, "/connections/nope/query", nil)
	req.SetPathValue("name", "nope")
	rec := httptest.NewRecorder()

	h.View(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestQueryHandler_Run_OQL(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/oql/query", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"data":   []map[string]any{{"id": float64(1), "name": "Alice"}},
			"stats":  map[string]any{"rows_scanned": 10, "rows_returned": 1},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(queryRunRequest{Mode: "oql", Query: "SELECT * FROM users"})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/run", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.Run(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotBody["query"] != "SELECT * FROM users" {
		t.Fatalf("upstream request = %+v, query not passed through correctly", gotBody)
	}
	if !strings.Contains(rec.Body.String(), "Alice") {
		t.Fatalf("body missing the query result: %s", rec.Body.String())
	}
	var got oqlRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if !got.Classification.IsSimpleSelect || got.Classification.SourceTable != "users" {
		t.Fatalf("classification = %+v, want a simple select on users for %q", got.Classification, "SELECT * FROM users")
	}
	// OQLResult's own fields (embedded by pointer) must still flatten
	// to the top level, unchanged from before classification existed.
	if got.OQLResult == nil || got.Status != "ok" || len(got.Data) != 1 {
		t.Fatalf("OQLResult fields not preserved at top level: %+v", got)
	}
}

func TestQueryHandler_Run_OQL_NonSimpleQueryClassifiedCorrectly(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/oql/query", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"data":   []map[string]any{{"name": "Alice", "total": float64(3)}},
			"stats":  map[string]any{"rows_scanned": 10, "rows_returned": 1},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(queryRunRequest{Mode: "oql", Query: "SELECT name, COUNT(*) AS total FROM users GROUP BY name"})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/run", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.Run(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got oqlRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if got.Classification.IsSimpleSelect {
		t.Fatalf("classification = %+v, want IsSimpleSelect=false for an aggregate/GROUP BY query", got.Classification)
	}
	if got.OQLResult == nil || len(got.Data) != 1 {
		t.Fatalf("OQLResult fields not preserved at top level: %+v", got)
	}
}

func TestQueryHandler_Run_Sulpher(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/graph/query", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"result": []map[string]any{{"path": "a->b"}},
			"stats":  map[string]any{"nodes_traversed": 2},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(queryRunRequest{Mode: "sulpher", Query: "MATCH (a)-->(b) RETURN a,b"})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/run", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.Run(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotBody["query"] != "MATCH (a)-->(b) RETURN a,b" {
		t.Fatalf("upstream request = %+v, query not passed through correctly", gotBody)
	}
	// maxDepth=0 (xolu's own server default) must not be sent at all —
	// GraphQuery's own contract: max_depth is only included when > 0.
	if _, present := gotBody["max_depth"]; present {
		t.Fatalf("upstream request included max_depth = %v, want it omitted for the default", gotBody["max_depth"])
	}
	if !strings.Contains(rec.Body.String(), "nodes_traversed") {
		t.Fatalf("body missing the query result: %s", rec.Body.String())
	}
	var got sulpherRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if got.GraphQueryResult == nil || got.Status != "ok" {
		t.Fatalf("GraphQueryResult fields not preserved at top level: %+v", got)
	}
	if len(got.GraphData.Nodes) != 0 || len(got.GraphData.Edges) != 0 {
		t.Fatalf("graphData = %+v, want empty for a scalar-shaped result ({\"path\": \"a->b\"} has no _id/from/rel/to markers)", got.GraphData)
	}
}

func TestQueryHandler_Run_Sulpher_GraphShapedResultClassified(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/graph/query", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"result": []map[string]any{
				{
					"a": map[string]any{"_id": float64(1), "type": "deals", "name": "Big Deal"},
					"b": map[string]any{"_id": float64(4), "type": "users", "name": "Alice"},
					"r": map[string]any{"from": "deals:1", "to": "users:4", "rel": "owner"},
				},
			},
			"stats": map[string]any{"nodes_traversed": 2},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(queryRunRequest{Mode: "sulpher", Query: "MATCH (a)-[r]->(b) RETURN a, r, b"})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/run", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.Run(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got sulpherRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if len(got.GraphData.Nodes) != 2 || len(got.GraphData.Edges) != 1 {
		t.Fatalf("graphData = %+v, want 2 nodes and 1 edge classified from the graph-shaped result", got.GraphData)
	}
}

func TestQueryHandler_Run_REST(t *testing.T) {
	var gotMethod, gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/bal/def", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"defs":[]}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(queryRunRequest{Mode: "rest", Method: "GET", Path: "/api/v1/bal/def"})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/run", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.Run(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotMethod != "GET" || gotPath != "/api/v1/bal/def" {
		t.Fatalf("upstream request = %s %s, unexpected", gotMethod, gotPath)
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if result["statusCode"] != float64(200) {
		t.Fatalf("statusCode = %v, want 200", result["statusCode"])
	}
	if !strings.Contains(result["body"].(string), "defs") {
		t.Fatalf("body field = %v, want the raw response body passed through", result["body"])
	}
}

func TestQueryHandler_Run_REST_NonOKStatusStillReturnsNormally(t *testing.T) {
	// Raw's own contract: a 4xx/5xx the server actually sent is not a
	// Go error — it's a normal *RawResult with StatusCode set. This
	// must surface as a normal 200-from-xoluman response carrying the
	// real status inside, not as an upstream error page.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/nonexistent", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(queryRunRequest{Mode: "rest", Method: "GET", Path: "/api/v1/nonexistent"})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/run", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.Run(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d — a real 404 from the target server is data, not a xoluman-level error", rec.Code, http.StatusOK)
	}
	var result map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &result)
	if result["statusCode"] != float64(404) {
		t.Fatalf("statusCode = %v, want 404 carried through in the result", result["statusCode"])
	}
}

func TestQueryHandler_Run_REST_MissingPathRejected(t *testing.T) {
	store := seedConnection(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).URL)
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(queryRunRequest{Mode: "rest", Method: "GET", Path: ""})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/run", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.Run(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestQueryHandler_Run_UnknownMode(t *testing.T) {
	store := seedConnection(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).URL)
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(queryRunRequest{Mode: "not-a-real-mode", Query: "x"})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/run", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.Run(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestQueryHandler_Run_MalformedBody(t *testing.T) {
	store := seedConnection(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).URL)
	h := &queryHandler{store: store}

	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/run", strings.NewReader("not json"))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.Run(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestQueryHandler_Run_UnknownConnection(t *testing.T) {
	h := &queryHandler{store: newTestStore(t)}
	reqBody, _ := json.Marshal(queryRunRequest{Mode: "oql", Query: "x"})
	req := httptest.NewRequest(http.MethodPost, "/connections/nope/query/run", bytes.NewReader(reqBody))
	req.SetPathValue("name", "nope")
	rec := httptest.NewRecorder()

	h.Run(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestQueryHandler_CreateGraphNode_SchemaFulSuccess(t *testing.T) {
	var createdBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/widgets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":       "object",
			"properties": map[string]any{"name": map[string]any{"type": "string"}},
			"required":   []string{"name"},
		})
	})
	mux.HandleFunc("POST /api/v1/widgets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&createdBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(42), "message": "created"})
	})
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "pagination": map[string]any{"total_pages": 1}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	form := url.Values{"name": {"Gadget"}}
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph/create/widgets", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.CreateGraphNode(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got createGraphNodeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if got.ID != 42 || got.Error != "" {
		t.Fatalf("got %+v, want ID=42 no error", got)
	}
	if createdBody["name"] != "Gadget" {
		t.Errorf("created body = %+v, want name=Gadget", createdBody)
	}
}

func TestQueryHandler_CreateGraphNode_MissingRequiredFieldRejected(t *testing.T) {
	var createCalled bool
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/widgets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":       "object",
			"properties": map[string]any{"name": map[string]any{"type": "string"}},
			"required":   []string{"name"},
		})
	})
	mux.HandleFunc("POST /api/v1/widgets", func(w http.ResponseWriter, r *http.Request) {
		createCalled = true
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(1)})
	})
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "pagination": map[string]any{"total_pages": 1}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	form := url.Values{"name": {""}} // present but empty -- what a real rendered <input name="name"> actually submits when left blank, not an omitted key
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph/create/widgets", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.CreateGraphNode(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
	if createCalled {
		t.Error("c.Create was called despite a client-side validation failure -- should have been rejected before ever reaching the network")
	}
	var got createGraphNodeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if got.Error == "" {
		t.Error("expected a non-empty error message")
	}
}

func TestQueryHandler_CreateGraphNode_UpstreamCreateFailureSurfaced(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/widgets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}})
	})
	mux.HandleFunc("POST /api/v1/widgets", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "duplicate"}})
	})
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "pagination": map[string]any{"total_pages": 1}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	form := url.Values{"name": {"Gadget"}}
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph/create/widgets", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.CreateGraphNode(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	var got createGraphNodeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if got.Error == "" {
		t.Error("expected the upstream error to be surfaced")
	}
}

func TestQueryHandler_CreateGraphNode_RefFieldReconstructed(t *testing.T) {
	var createdBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/tasks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{"type": "string"},
				"owner": map[string]any{"type": "object", "format": "ref", "target": "users"},
			},
			"required": []string{"title"},
		})
	})
	mux.HandleFunc("POST /api/v1/tasks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&createdBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(9)})
	})
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "pagination": map[string]any{"total_pages": 1}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	form := url.Values{"title": {"Follow up"}, "owner": {"4"}}
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph/create/tasks", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "tasks")
	rec := httptest.NewRecorder()

	h.CreateGraphNode(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	owner, ok := createdBody["owner"].(map[string]any)
	if !ok || owner["type"] != "REF" || owner["entity"] != "users" || owner["id"] != float64(4) {
		t.Fatalf("created owner field = %+v, want a structured REF object -- the same reconstruction the entity form and grid editor already do", createdBody["owner"])
	}
}

func TestQueryHandler_CreateGraphNode_UnknownConnection(t *testing.T) {
	store := seedConnection(t, "http://unused.invalid")
	h := &queryHandler{store: store}

	req := httptest.NewRequest(http.MethodPost, "/connections/does-not-exist/query/graph/create/widgets", strings.NewReader(""))
	req.SetPathValue("name", "does-not-exist")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.CreateGraphNode(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestQueryHandler_RefFieldsForType_KnownAndUnknownTargets(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/deals", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":    map[string]any{"type": "string"},
				"owner":   map[string]any{"type": "object", "format": "ref", "target": "users"},
				"company": map[string]any{"type": "object", "format": "ref"}, // no declared target
			},
		})
	})
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "pagination": map[string]any{"total_pages": 1}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/query/graph/ref-fields/deals", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "deals")
	rec := httptest.NewRecorder()

	h.RefFieldsForType(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got struct {
		Fields    []refFieldInfo `json:"fields"`
		HasSchema bool           `json:"hasSchema"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if !got.HasSchema {
		t.Error("HasSchema = false, want true for a real, schema-ful type")
	}
	if len(got.Fields) != 2 {
		t.Fatalf("fields = %+v, want exactly 2 (name is not a REF field, should be excluded)", got.Fields)
	}
	byName := map[string]refFieldInfo{}
	for _, f := range got.Fields {
		byName[f.Name] = f
	}
	if byName["owner"].Target != "users" {
		t.Errorf("owner target = %q, want users", byName["owner"].Target)
	}
	if byName["company"].Target != "" {
		t.Errorf("company target = %q, want empty (no declared target, none remembered)", byName["company"].Target)
	}
}

func TestQueryHandler_RefFieldsForType_SchemaLessTypeReturnsEmptyFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/blobs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/query/graph/ref-fields/blobs", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "blobs")
	rec := httptest.NewRecorder()

	h.RefFieldsForType(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got struct {
		Fields    []refFieldInfo `json:"fields"`
		HasSchema bool           `json:"hasSchema"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if got.HasSchema {
		t.Error("HasSchema = true, want false for a genuinely schema-less type")
	}
	if len(got.Fields) != 0 {
		t.Fatalf("fields = %+v, want empty for a schema-less type", got.Fields)
	}
}

func TestQueryHandler_RefFieldsForType_UnknownConnection(t *testing.T) {
	store := seedConnection(t, "http://unused.invalid")
	h := &queryHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/does-not-exist/query/graph/ref-fields/deals", nil)
	req.SetPathValue("name", "does-not-exist")
	req.SetPathValue("type", "deals")
	rec := httptest.NewRecorder()

	h.RefFieldsForType(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
