// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"bytes"
	"encoding/json"
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
