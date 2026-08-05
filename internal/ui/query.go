// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	mi "github.com/ha1tch/minty"
	xclient "github.com/ha1tch/xolu/pkg/client"

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
			mux.HandleFunc("GET /connections/{name}/query/saved", h.ListSavedQueries)
			mux.HandleFunc("POST /connections/{name}/query/saved", h.CreateSavedQuery)
			mux.HandleFunc("DELETE /connections/{name}/query/saved/{id}", h.DeleteSavedQuery)
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

	basePath := "/connections/" + url.PathEscape(name) + "/query"
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

// savedQueryEntityType is a schema-less bookkeeping entity type,
// same established pattern as xoluman_field_meta (internal/fieldmeta)
// and xoluman_blob_folder (internal/blobfs) — persisted in the
// connected xolu instance itself, not xoluman's own local storage, so
// a saved query is visible to anyone else using xoluman against the
// same instance, matching the same reasoning those two already
// settled on: xoluman's own bookkeeping data lives where the data it
// describes lives, not in a separate local file only this one
// installation can see.
const savedQueryEntityType = "xoluman_saved_query"

// maxSavedQueryRows bounds a single fetch — a bookkeeping-scale table,
// same reasoning and same ceiling as fieldmeta's own maxFolderRows.
const maxSavedQueryRows = 1000

type savedQuery struct {
	ID          int64  `json:"id"`
	Mode        string `json:"mode"`
	Name        string `json:"name"`
	Query       string `json:"query,omitempty"`       // oql/sulpher only
	Method      string `json:"method,omitempty"`      // rest only
	Path        string `json:"path,omitempty"`        // rest only
	ContentType string `json:"contentType,omitempty"` // rest only
	Body        string `json:"body,omitempty"`        // rest only
	CreatedAt   string `json:"createdAt,omitempty"`
}

// ListSavedQueries returns every saved query for the given mode,
// newest first. mode is a required query parameter — the three modes
// are entirely separate sets from the UI's own perspective, and a
// client asking for "saved queries" without saying which mode wants a
// filtered list, not everything mixed together.
func (h *queryHandler) ListSavedQueries(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	mode := r.URL.Query().Get("mode")
	if mode != "oql" && mode != "sulpher" && mode != "rest" {
		http.Error(w, `mode must be "oql", "sulpher", or "rest"`, http.StatusBadRequest)
		return
	}
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

	result, err := c.List(r.Context(), savedQueryEntityType, &xclient.ListParams{Limit: maxSavedQueryRows})
	if err != nil {
		if xoluErr, ok := err.(*xclient.Error); ok && xoluErr.HTTPStatus == 404 {
			// Entity type doesn't exist yet — nobody has saved a query
			// against this instance at all. An empty list, not an error.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]savedQuery{})
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	queries := make([]savedQuery, 0, len(result.Entities))
	for _, e := range result.Entities {
		if s, ok := decodeSavedQuery(e); ok && s.Mode == mode {
			queries = append(queries, s)
		}
	}
	sort.Slice(queries, func(i, j int) bool { return queries[i].CreatedAt > queries[j].CreatedAt })

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(queries)
}

// CreateSavedQuery saves one query. name is required and must be
// non-empty — an unnamed saved query would be indistinguishable from
// every other one in the listbox this feeds.
func (h *queryHandler) CreateSavedQuery(w http.ResponseWriter, r *http.Request) {
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

	var req savedQuery
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Mode != "oql" && req.Mode != "sulpher" && req.Mode != "rest" {
		http.Error(w, `mode must be "oql", "sulpher", or "rest"`, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	data := map[string]any{
		"mode":       req.Mode,
		"name":       req.Name,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	}
	switch req.Mode {
	case "rest":
		data["method"] = req.Method
		data["path"] = req.Path
		data["content_type"] = req.ContentType
		data["body"] = req.Body
	default:
		data["query"] = req.Query
	}

	entity, err := c.Create(r.Context(), savedQueryEntityType, data)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": entity.ID})
}

// DeleteSavedQuery removes one saved query by id.
func (h *queryHandler) DeleteSavedQuery(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
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

	if err := c.Delete(r.Context(), savedQueryEntityType, id); err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeSavedQuery(e xclient.Entity) (savedQuery, bool) {
	s := savedQuery{ID: e.ID}
	mode, _ := e.Data["mode"].(string)
	nm, _ := e.Data["name"].(string)
	if mode == "" || nm == "" {
		return savedQuery{}, false
	}
	s.Mode = mode
	s.Name = nm
	s.Query, _ = e.Data["query"].(string)
	s.Method, _ = e.Data["method"].(string)
	s.Path, _ = e.Data["path"].(string)
	s.ContentType, _ = e.Data["content_type"].(string)
	s.Body, _ = e.Data["body"].(string)
	s.CreatedAt, _ = e.Data["created_at"].(string)
	return s, true
}
