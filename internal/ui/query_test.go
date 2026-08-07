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

func TestQueryHandler_Graph_ClassifiesNodesAndEdges(t *testing.T) {
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

	h.Graph(rec, req)

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

func TestQueryHandler_Graph_DeduplicatesNodesAndEdgesAcrossRows(t *testing.T) {
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

	h.Graph(rec, req)

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

func TestQueryHandler_Graph_SkipsScalarValues(t *testing.T) {
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

	h.Graph(rec, req)

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
	mux.HandleFunc("POST /api/v1/graph/query", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "result": []map[string]any{}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	reqBody, _ := json.Marshal(queryRunRequest{Query: "MATCH (d:deals) WHERE false RETURN d"})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/query/graph", bytes.NewReader(reqBody))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.Graph(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, "null") {
		t.Fatalf("body = %q, want empty arrays not null (the JS side does .map/.forEach on these directly)", body)
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
