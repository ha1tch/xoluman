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
