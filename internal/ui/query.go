// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	mi "github.com/ha1tch/minty"

	"github.com/ha1tch/xoluman/internal/connstore"
	"github.com/ha1tch/xoluman/internal/modules"
	"github.com/ha1tch/xoluman/internal/xoluext"
)

// RegisterQueryModule registers the query editor's routes onto reg,
// backed by store. No standalone nav entry, same reasoning as Entities
// and Blobs — reached per-connection.
func RegisterQueryModule(reg *modules.Registry, store connstore.Store) {
	h := &queryHandler{store: store}
	reg.Register(modules.Module{
		ID: "query",
		MountRoutes: func(mux *http.ServeMux) {
			mux.HandleFunc("GET /connections/{name}/query", h.View)
			mux.HandleFunc("POST /connections/{name}/query/run", h.Run)
		},
	})
}

type queryHandler struct {
	store connstore.Store
}

// View renders the query editor page — the Lit-shelled CodeMirror
// component, with a mode switcher (OQL/Sulpher/REST) and a results
// area the component itself populates via fetch to Run, not a page
// reload.
func (h *queryHandler) View(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := h.store.Get(r.Context(), name); err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	basePath := "/connections/" + name + "/query"
	body := func(b *mi.Builder) mi.Node {
		return b.Div(
			b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white mb-4"), "Query "+name),
			mi.Raw(`<xolu-query-editor run-url="`+htmlEscape(basePath+"/run")+`"></xolu-query-editor>`),
		)
	}
	WriteHTML(w, queryPage("Query "+name, r.URL.Path, body))
}

// queryPage wraps body in the shared shell plus the CodeMirror bundle
// (a plain ES module — no import map entry strictly needed for it
// since query-editor.js can import it by relative static path
// directly, but registered anyway for the same "imports" consistency
// as Lit) and the query-editor.js Lit shell as a module script.
func queryPage(title, activePath string, body mi.H) mi.H {
	return func(b *mi.Builder) mi.Node {
		extraHead := []mi.Node{
			b.Script(mi.Attr("type", "importmap"), mi.Raw(`{"imports":{"lit":"/static/vendor/lit@3.js","codemirror-bundle":"/static/vendor/codemirror-bundle@1.js"}}`)),
			b.Script(mi.Attr("type", "module"), mi.Attr("src", "/static/js/query-editor.js")),
		}
		return PageWithHead(title, activePath, extraHead, body)(b)
	}
}

// queryRunRequest is what the Lit component POSTs to Run.
type queryRunRequest struct {
	Mode  string `json:"mode"` // "oql", "sulpher", or "rest"
	Query string `json:"query"`
	// REST mode only:
	Method      string `json:"method,omitempty"`
	Path        string `json:"path,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Body        string `json:"body,omitempty"`
}

// Run executes a query in the requested mode and returns the raw
// result as JSON — the Lit component renders it, this handler doesn't
// shape the result into any particular display form, since OQL/Sulpher/
// REST results have genuinely different natural shapes (rows+stats,
// graph result+stats, arbitrary status+body+headers respectively).
func (h *queryHandler) Run(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	conn, err := h.store.Get(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}
	c := xoluext.BuildClient(conn)

	var req queryRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch strings.ToLower(req.Mode) {
	case "oql":
		result, err := c.OQL(r.Context(), req.Query)
		if err != nil {
			writeJSONError(w, err, name)
			return
		}
		_ = json.NewEncoder(w).Encode(result)

	case "sulpher":
		result, err := c.GraphQuery(r.Context(), req.Query, 0) // 0 = xolu's own server default
		if err != nil {
			writeJSONError(w, err, name)
			return
		}
		_ = json.NewEncoder(w).Encode(result)

	case "rest":
		if req.Path == "" || req.Path[0] != '/' {
			http.Error(w, `path must start with "/"`, http.StatusBadRequest)
			return
		}
		method := req.Method
		if method == "" {
			method = http.MethodGet
		}
		var body io.Reader
		if req.Body != "" {
			body = strings.NewReader(req.Body)
		}
		result, err := c.Raw(r.Context(), method, req.Path, req.ContentType, body)
		if err != nil {
			// Raw's own contract: a non-nil error here means the
			// request never completed (transport failure) — a real
			// 4xx/5xx the server actually sent comes back as a
			// non-nil *RawResult instead, handled below, not here.
			writeJSONError(w, err, name)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": result.StatusCode,
			"body":       string(result.Body),
			"headers":    result.Header,
		})

	default:
		http.Error(w, `mode must be "oql", "sulpher", or "rest"`, http.StatusBadRequest)
	}
}

func htmlEscape(s string) string {
	r := strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", `<`, "&lt;", `>`, "&gt;")
	return r.Replace(s)
}
