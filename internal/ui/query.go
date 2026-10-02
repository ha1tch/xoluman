// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/ha1tch/xoluman/internal/formengine"
	"github.com/ha1tch/xoluman/internal/modules"
	"github.com/ha1tch/xoluman/internal/oqlclassify"
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
			mux.HandleFunc("POST /connections/{name}/query/graph", h.Graph)
			mux.HandleFunc("GET /connections/{name}/query/graph/entity-types", h.GraphEntityTypes)
			mux.HandleFunc("POST /connections/{name}/query/graph/expand", h.ExpandGraphNode)
			mux.HandleFunc("POST /connections/{name}/query/graph/create/{type}", h.CreateGraphNode)
			mux.HandleFunc("GET /connections/{name}/query/graph/ref-fields/{type}", h.RefFieldsForType)
			mux.HandleFunc("POST /connections/{name}/query/graph/node", h.SaveGraphNode)
			mux.HandleFunc("POST /connections/{name}/query/graph/edge", h.SaveGraphEdge)
			mux.HandleFunc("POST /connections/{name}/dxp/preset-run", h.RunDXPPreset)
			mux.HandleFunc("GET /connections/{name}/graph", h.GraphView)
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
			mi.Raw(`<xolu-query-editor run-url="`+htmlEscape(basePath+"/run")+`" modes="oql,rest"></xolu-query-editor>`),
		)
	}
	WriteHTML(w, queryPage("Query "+name, r.URL.Path, body))
}

// GraphView renders the graph viewer page — the same fsm-editor.js
// Lit shell used for machine definitions, in mode="graph" (see that
// file's own top comment on the split): a Sulpher query input instead
// of the FSM name/state/transition fields, and click-to-inspect
// instead of validation/save. Deliberately reuses fsmPage (loads the
// same fsm-canvas-engine.js + fsm-editor.js scripts) rather than a
// parallel page-shell function — this is one widget with two modes,
// not two widgets.
func (h *queryHandler) GraphView(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := h.store.Get(r.Context(), name); err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	graphURL := "/connections/" + url.PathEscape(name) + "/query/graph"
	entityTypesURL := graphURL + "/entity-types"
	entitiesBaseURL := "/connections/" + url.PathEscape(name) + "/entities"
	runURL := "/connections/" + url.PathEscape(name) + "/query/run"

	body := func(b *mi.Builder) mi.Node {
		// Graph view is the default, initially-visible tab
		// deliberately — fsm-editor.js's own initial sizing reads the
		// canvas-wrap's real, rendered bounding box (zoomToFit,
		// ResizeObserver setup) at connect time, which would see 0×0
		// if this panel started display:none. query-editor.js has no
		// equivalent concern (CodeMirror lays out reactively
		// regardless of prior visibility), so the Sulpher panel
		// starting hidden is safe the other way around.
		return mi.Raw(`
			<div class="xolu-graph-page-tabs">
				<div class="xolu-graph-page-tab active" data-tab="graph">Graph view</div>
				<div class="xolu-graph-page-tab" data-tab="sulpher">Sulpher</div>
			</div>
			<div class="xolu-graph-page-panel" data-panel="graph">
				<xolu-fsm-editor mode="graph" graph-url="` + htmlEscape(graphURL) + `" graph-entity-types-url="` + htmlEscape(entityTypesURL) + `" entities-base-url="` + htmlEscape(entitiesBaseURL) + `"></xolu-fsm-editor>
			</div>
			<div class="xolu-graph-page-panel" data-panel="sulpher" style="display:none">
				<xolu-query-editor run-url="` + htmlEscape(runURL) + `" modes="sulpher"></xolu-query-editor>
			</div>
			<style>
				.xolu-graph-page-tabs { display:flex; gap:4px; margin-bottom:0.5rem; }
				.xolu-graph-page-tab { padding:0.4rem 0.9rem; font-size:0.8125rem; font-weight:500; border-radius:0.375rem 0.375rem 0 0; cursor:pointer; color:#6b7280; }
				.xolu-graph-page-tab.active { background:#4f46e5; color:#fff; }
				.dark .xolu-graph-page-tab { color:#9ca3af; }
			</style>
			<script>
				(function() {
					var tabs = document.querySelectorAll('.xolu-graph-page-tab');
					var panels = document.querySelectorAll('.xolu-graph-page-panel');
					function activate(name) {
						tabs.forEach(function(t) { t.classList.toggle('active', t.dataset.tab === name); });
						panels.forEach(function(p) { p.style.display = p.dataset.panel === name ? '' : 'none'; });
					}
					tabs.forEach(function(t) { t.addEventListener('click', function() { activate(t.dataset.tab); }); });
					// The Sulpher tab's own "View in graph" button
					// (query-editor.js's own _viewInGraph) dispatches
					// this — the bridge between the two sibling
					// components lives here, at the page level, not
					// inside either component, since neither one
					// should need to know the other exists.
					document.addEventListener('xolu-view-in-graph', function(e) {
						activate('graph');
						var fsmEditor = document.querySelector('xolu-fsm-editor');
						if (fsmEditor) fsmEditor.applyExternalGraphData(e.detail);
					});
				})();
			</script>
		`)
	}
	WriteHTML(w, graphPage("Graph — "+name, r.URL.Path, body))
}

// graphPage combines both script sets the graph view's two tabs need:
// fsm-canvas-engine.js + fsm-editor.js for the graph-walker tab (same
// as fsmPage's own setup), plus codemirror-bundle + query-editor.js
// for the Sulpher tab living beside it — the graph view is the one
// page in xoluman that needs both.
func graphPage(title, activePath string, body mi.H) mi.H {
	return func(b *mi.Builder) mi.Node {
		extraHead := []mi.Node{
			b.Script(mi.Attr("type", "importmap"), mi.Raw(`{"imports":{"lit":"/static/vendor/lit@3.js","codemirror-bundle":"/static/vendor/codemirror-bundle@1.js"}}`)),
			// Plain script, not a module — see fsmPage's own comment
			// for why fsm-canvas-engine.js needs to execute before the
			// deferred module script that calls its globals.
			b.Script(mi.Attr("src", "/static/js/fsm-canvas-engine.js")),
			b.Script(mi.Attr("type", "module"), mi.Attr("src", "/static/js/fsm-editor.js")),
			b.Script(mi.Attr("type", "module"), mi.Attr("src", "/static/js/query-editor.js")),
		}
		return PageWithHead(title, activePath, extraHead, body)(b)
	}
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

// sulpherRunResponse wraps xolu's own GraphQueryResult, adding the
// same classified {nodes, edges} shape graphSulpherDormant already
// computes — the query view's own Sulpher tab uses GraphData.Nodes'
// own length to decide whether to offer "view in graph" at all.
// GraphQueryResult is embedded by pointer for the same reason
// oqlRunResponse embeds OQLResult: its own fields (Status/Result/
// Stats) flatten to the top level unchanged from before this existed.
type sulpherRunResponse struct {
	*xclient.GraphQueryResult
	GraphData graphData `json:"graphData"`
}

// oqlRunResponse wraps xolu's own OQLResult, adding classification
// info the JS side uses to offer "view as editable grid" (only for a
// genuine simple select) or a generic, read-only "view as table"
// (any query whose own result rows are already a non-empty array —
// the JS side's own concern, this response just supplies the
// classification). OQLResult is embedded by pointer specifically so
// its own fields (Status/Data/Stats) flatten into the top-level JSON
// exactly as before this change — nothing that already reads
// result.data or result.stats needs to change.
type oqlRunResponse struct {
	*xclient.OQLResult
	Classification oqlClassificationInfo `json:"classification"`
}

type oqlClassificationInfo struct {
	IsSimpleSelect bool   `json:"isSimpleSelect"`
	SourceTable    string `json:"sourceTable,omitempty"`
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
		// Classification is best-effort and never blocks the real
		// result: a query that executed successfully against xolu but
		// happens to be unparseable by oqlclassify (a genuine gap in
		// this session's own classifier, or an OQL dialect extension
		// it doesn't yet know) still returns its own real data —
		// IsSimpleSelect just stays false, the same as any other
		// non-simple query, rather than failing the whole request.
		class, _ := oqlclassify.Classify(req.Query)
		_ = json.NewEncoder(w).Encode(oqlRunResponse{
			OQLResult:      result,
			Classification: oqlClassificationInfo{IsSimpleSelect: class.IsSimpleSelect, SourceTable: class.SourceTable},
		})

	case "sulpher":
		result, err := c.GraphQuery(r.Context(), req.Query, 0) // 0 = xolu's own server default
		if err != nil {
			writeJSONError(w, err, name)
			return
		}
		// graphData is included unconditionally, even when empty — a
		// scalar/aggregate Sulpher query (COUNT(*), a single property
		// return) has nothing to draw, and an empty {nodes: [],
		// edges: []} is exactly how the JS side already tells "no
		// graph-shaped data here" from "there's a graph to view" (it
		// checks graphData.nodes.length, not whether the field is
		// present at all).
		_ = json.NewEncoder(w).Encode(sulpherRunResponse{
			GraphQueryResult: result,
			GraphData:        classifySulpherResult(result.Result),
		})

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

	// dxp only — see internal/ui/dxppreset.go's own doc comment on
	// what these mean and why a preset carries a resolver *name*
	// (looked up in an in-code registry) rather than any executable
	// logic of its own.
	DxpDefName string               `json:"dxp_def_name,omitempty"`
	Resolver   string               `json:"resolver,omitempty"`
	FormFields []dxpPresetFormField `json:"form_fields,omitempty"`
}

// ListSavedQueries returns every saved query for the given mode,
// newest first. mode is a required query parameter — the three modes
// are entirely separate sets from the UI's own perspective, and a
// client asking for "saved queries" without saying which mode wants a
// filtered list, not everything mixed together.
func (h *queryHandler) ListSavedQueries(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	mode := r.URL.Query().Get("mode")
	if mode != "oql" && mode != "sulpher" && mode != "rest" && mode != "dxp" {
		http.Error(w, `mode must be "oql", "sulpher", "rest", or "dxp"`, http.StatusBadRequest)
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

	entities, err := xoluext.ListAll(r.Context(), c, savedQueryEntityType)
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

	queries := make([]savedQuery, 0, len(entities))
	for _, e := range entities {
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
	if req.Mode != "oql" && req.Mode != "sulpher" && req.Mode != "rest" && req.Mode != "dxp" {
		http.Error(w, `mode must be "oql", "sulpher", "rest", or "dxp"`, http.StatusBadRequest)
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
	case "dxp":
		data["dxp_def_name"] = req.DxpDefName
		data["resolver"] = req.Resolver
		data["form_fields"] = req.FormFields
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
// graphNode and graphEdge are the shape the graph-mode fsm-editor.js
// widget actually consumes — deliberately not xolu's own raw Sulpher
// row shape, which mixes entity documents and relationship markers
// together per-row with duplicates across rows. ID is "type:id",
// matching the same format Sulpher's own edge from/to fields already
// use (confirmed directly against a real query response, not assumed
// from source) — so an edge's From/To can be used as a node lookup
// key with no extra parsing.
// parseGraphNodeID splits a "type:id" reference (the format both
// graphNode.ID and graphEdge.From/To already use — see the Graph
// handler's own doc comment on where that shape comes from) into its
// two parts. The type may itself contain no colon (entity type names
// don't), so a plain split on the first colon is sufficient and
// unambiguous.
func parseGraphNodeID(ref string) (entityType string, id int64, err error) {
	idx := strings.IndexByte(ref, ':')
	if idx < 0 {
		return "", 0, fmt.Errorf("malformed node reference %q, want \"type:id\"", ref)
	}
	entityType = ref[:idx]
	id, err = strconv.ParseInt(ref[idx+1:], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("malformed node reference %q: %w", ref, err)
	}
	return entityType, id, nil
}

type graphNodeSaveRequest struct {
	Type    string         `json:"type"`
	ID      int64          `json:"id"`
	Changes map[string]any `json:"changes"`
}

// SaveGraphNode is the write side of the graph viewer's inspect
// panel — a plain xolu Patch against whichever entity the clicked
// node actually is, the same partial-update mechanism the grid editor
// already uses (see GridSave in grid.go). Scalar fields only in
// practice: the JS side deliberately doesn't offer editing for REF-
// typed fields here (see fsm-editor.js's own comment on why) — those
// go through SaveGraphEdge instead, where the relationship itself is
// the thing being changed, not a side effect of editing a form field
// that happens to hold a REF.
func (h *queryHandler) SaveGraphNode(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	conn, err := h.store.Get(r.Context(), name)
	if err != nil {
		writeJSONError(w, err, name)
		return
	}
	c := xoluext.BuildClient(conn)

	var req graphNodeSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Type == "" || req.ID <= 0 {
		http.Error(w, "type and a positive id are required", http.StatusBadRequest)
		return
	}

	if _, err := c.Patch(r.Context(), req.Type, req.ID, req.Changes); err != nil {
		writeJSONError(w, err, name)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type graphEdgeSaveRequest struct {
	From     string `json:"from"`     // "type:id" -- the source entity whose REF field is being changed
	RelField string `json:"relField"` // the field name on the source entity to update (matches the edge's own "rel")
	NewTo    string `json:"newTo"`    // "type:id" -- the new target
}

// SaveGraphEdge retargets a relationship — since xolu's graph model
// derives edges from a REF field on the source entity rather than a
// first-class relationship object with its own properties (confirmed
// directly against real query output when this endpoint's own read
// side, Graph, was first built — see that handler's doc comment),
// "editing an edge" means patching the source entity's own REF field
// to point somewhere else. The target entity type is not user-
// supplied here — it's read back from the edge being edited, since
// retargeting a REF field to a different entity type than the field
// was already pointing at is not a relationship edit, it's a schema
// violation, and this endpoint should not be the place that allows it
// silently.
func (h *queryHandler) SaveGraphEdge(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	conn, err := h.store.Get(r.Context(), name)
	if err != nil {
		writeJSONError(w, err, name)
		return
	}
	c := xoluext.BuildClient(conn)

	var req graphEdgeSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.RelField == "" {
		http.Error(w, "relField is required", http.StatusBadRequest)
		return
	}

	fromType, fromID, err := parseGraphNodeID(req.From)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// An explicitly empty newTo clears the relationship rather than
	// retargeting it — confirmed directly against a real xolu instance
	// that PATCH with a null value genuinely removes the field (not
	// just stores a literal null): a follow-up GET showed the field
	// absent entirely. Previously unreachable (parseGraphNodeID would
	// have rejected an empty string as an invalid node ID before this
	// branch existed), so this is purely additive.
	if req.NewTo == "" {
		if _, err := c.Patch(r.Context(), fromType, fromID, map[string]any{req.RelField: nil}); err != nil {
			writeJSONError(w, err, name)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	toType, toID, err := parseGraphNodeID(req.NewTo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	refValue := map[string]any{"entity": toType, "id": toID, "type": "REF"}
	if _, err := c.Patch(r.Context(), fromType, fromID, map[string]any{req.RelField: refValue}); err != nil {
		writeJSONError(w, err, name)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// graphNode and graphEdge are the shape the graph-mode fsm-editor.js
// widget actually consumes — deliberately not xolu's own raw Sulpher
// row shape, which mixes entity documents and relationship markers
// together per-row with duplicates across rows. ID is "type:id",
// matching the same format Sulpher's own edge from/to fields already
// use (confirmed directly against a real query response, not assumed
// from source) — so an edge's From/To can be used as a node lookup
// key with no extra parsing.
type graphNode struct {
	ID   string         `json:"id"`
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
	// Collapsed marks a node whose own REF fields were never walked —
	// discovered only as another node's edge target, with no hop
	// budget (or fetch-ceiling budget) left to fetch and expand it.
	// Data for a collapsed node is minimal (just its id) rather than
	// the full entity document. The JS side renders these with the
	// same double-circle style fsm-canvas-engine.js already uses for
	// FSM accept/terminal states (isAcceptState) — reused, not
	// reimplemented — and offers an explicit expand action
	// (POST .../graph/expand) that fetches the real document and
	// walks its own REFs, the same restEmbedGraphRunner logic Run
	// itself uses, just seeded from one already-known node instead of
	// a fresh entity-type list.
	Collapsed bool `json:"collapsed,omitempty"`
}

type graphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Rel  string `json:"rel"`
}

type graphData struct {
	Nodes []graphNode `json:"nodes"`
	Edges []graphEdge `json:"edges"`
}

// graphSulpherDormant is the original graph query implementation —
// MATCH ... RETURN against xolu's /graph/query endpoint. Dormant, not
// deleted: blocked end to end by XM-9 (whole-node RETURN failing
// against schema-adapted entities — xoluman-xolu-consolidated-report-
// log.md) for every entity type this session's own CRM example uses,
// since all of them have a schema registered. graphrest.go's
// restEmbedGraphRunner is what Graph (below) actually calls today,
// via plain REST + embed_depth instead of Sulpher. Swapping back once
// XM-9 is resolved means changing Graph's own body to call this
// function's logic (or reactivating it directly) instead of
// constructing a restEmbedGraphRunner — the {nodes, edges} contract
// (graphNode, graphEdge, graphData, all defined below) and the JS
// graph viewer's own consumption of it are both unaffected either way.
//
// Kept as a full, working method (not deleted) specifically so it
// stays buildable and reviewable — reshapes a Sulpher graph-query
// result into {nodes, edges} by classifying each value in each
// returned row by the markers Sulpher's own executor actually emits
// (confirmed directly, not guessed): a node has both "_id" and "type";
// an edge has "from"/"rel"/"to" and no "_id". Anything else (a scalar
// from an aggregate RETURN, for instance) is skipped, not an error —
// a graph query mixing COUNT(*) with node variables is valid Sulpher,
// just not something this view can draw.
func (h *queryHandler) graphSulpherDormant(w http.ResponseWriter, r *http.Request) {
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

	result, err := c.GraphQuery(r.Context(), req.Query, 0)
	if err != nil {
		writeJSONError(w, err, name)
		return
	}

	data := classifySulpherResult(result.Result)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

// classifySulpherResult reshapes a raw Sulpher GraphQueryResult.Result
// into {nodes, edges} — one pass over every value in every returned
// row, classifying each by the markers Sulpher's own executor actually
// emits (confirmed directly, not guessed): a node has both "_id" and
// "type"; an edge has "from"/"rel"/"to" and no "_id". Anything else (a
// scalar from an aggregate RETURN, for instance) is skipped, not an
// error — a graph query mixing COUNT(*) with node variables is valid
// Sulpher, just not something this can draw. Shared between
// graphSulpherDormant (the dormant standalone endpoint) and Run's own
// "sulpher" case (which enriches the raw result with this same
// classification so the query view's own Sulpher tab can offer "view
// in graph" without a second round trip) — extracted specifically so
// both stay identical by construction, not by convention.
func classifySulpherResult(rows []map[string]any) graphData {
	data := graphData{Nodes: []graphNode{}, Edges: []graphEdge{}}
	seenNodes := map[string]bool{}
	seenEdges := map[string]bool{}
	for _, row := range rows {
		for _, v := range row {
			m, ok := v.(map[string]any)
			if !ok {
				continue
			}
			if id, hasID := m["_id"]; hasID {
				typ, _ := m["type"].(string)
				key := typ + ":" + fmt.Sprintf("%v", id)
				if seenNodes[key] {
					continue
				}
				seenNodes[key] = true
				data.Nodes = append(data.Nodes, graphNode{ID: key, Type: typ, Data: m})
				continue
			}
			from, hasFrom := m["from"].(string)
			to, hasTo := m["to"].(string)
			rel, _ := m["rel"].(string)
			if hasFrom && hasTo {
				key := from + "|" + rel + "|" + to
				if seenEdges[key] {
					continue
				}
				seenEdges[key] = true
				data.Edges = append(data.Edges, graphEdge{From: from, To: to, Rel: rel})
			}
		}
	}
	return data
}

// graphRunRequest is what the graph viewer's UI actually sends today —
// no query text at all, just which entity type to start exploring
// from and how many REF hops to follow. Replaces the free-text Sulpher
// query the JS used to send while graphSulpherDormant was live; see
// its own doc comment for why.
type graphRunRequest struct {
	EntityType string `json:"entityType"`
	Depth      int    `json:"depth"`
	Limit      int    `json:"limit"`
}

// Graph builds {nodes, edges} via restEmbedGraphRunner (graphrest.go)
// — plain REST calls with explicit embed_depth, no query language.
// See graphSulpherDormant's own doc comment for the dormant
// Sulpher-based alternative this replaced, and graphrest.go's Run for
// why the REST approach is designed the way it is.
func (h *queryHandler) Graph(w http.ResponseWriter, r *http.Request) {
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

	var req graphRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	data, err := (restEmbedGraphRunner{}).Run(r.Context(), c, conn, restGraphSpec{
		EntityType: req.EntityType, EmbedDepth: req.Depth, Limit: req.Limit,
	})
	if err != nil {
		writeJSONError(w, err, name)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

// GraphEntityTypes lists every entity type with actual data on this
// instance — the graph viewer's own starting-point picker, same
// source (client.ListEntities) as the plain entities list page uses
// for its own type list, exposed here as JSON since the graph viewer
// is a Lit component fetching its own data, not a server-rendered page.
func (h *queryHandler) GraphEntityTypes(w http.ResponseWriter, r *http.Request) {
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

	entries, err := c.ListEntities(r.Context(), false)
	if err != nil {
		writeJSONError(w, err, name)
		return
	}
	types := make([]string, len(entries))
	for i, e := range entries {
		types[i] = e.EntityType
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(types)
}

// createGraphNodeResponse is what CreateGraphNode returns — success
// (a real ID) or a plain error message. Deliberately simple: the form
// itself (fetched from the real, unmodified NewForm, see JS side)
// already shows the person's own typed values; a re-rendered,
// field-annotated error form is a real refinement but not needed for
// a first version, matching the same "start simple, refine later"
// permission this whole feature was designed under.
type createGraphNodeResponse struct {
	ID    int64  `json:"id,omitempty"`
	Error string `json:"error,omitempty"`
}

// CreateGraphNode is the graph viewer's own way of creating a brand-
// new entity — the answer to double-click (a local-only, unpersisted
// node) followed by right-click, picking an entity type, and filling
// in the resulting form. Deliberately not a reuse of entities.go's
// own Create: that handler redirects on success (fine for its own
// full-page-navigation flow, useless here — there is no new ID
// anywhere in a redirect target to read back and attach to the graph
// node), so this is a small, separate handler built from the exact
// same underlying pieces (resolveEntityFields, refTargetsForSubmission,
// formengine.ParseFormValues, rememberNewRefTargets) with a JSON
// response shaped for what an AJAX caller actually needs. The form
// markup itself is not duplicated anywhere — the JS side fetches the
// real, unmodified NewForm fragment (via the HX-Request header
// WriteModalAware already understands) and posts its own field values
// here instead of to NewForm's own action.
func (h *queryHandler) CreateGraphNode(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entityType := r.PathValue("type")
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

	fields, schema, ok, err := resolveEntityFields(r.Context(), c, entityType, nil)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}
	if !ok {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(createGraphNodeResponse{Error: entityType + " has no registered schema and no existing rows to infer fields from"})
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	refTargets := refTargetsForSubmission(r.Context(), c, schema)
	values, errs := formengine.ParseFormValues(fields, r.PostForm, refTargets)
	if len(errs) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Header().Set("Content-Type", "application/json")
		msg := ""
		for field, e := range errs {
			msg += field + ": " + e + "; "
		}
		_ = json.NewEncoder(w).Encode(createGraphNodeResponse{Error: msg})
		return
	}

	created, err := c.Create(r.Context(), entityType, values)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(createGraphNodeResponse{Error: err.Error()})
		return
	}
	rememberNewRefTargets(r.Context(), c, entityType, schema, r.PostForm, refTargets)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(createGraphNodeResponse{ID: created.ID})
}

// graphExpandRequest is what the JS side sends when someone clicks a
// collapsed node (see graphNode.Collapsed's own doc comment) and asks
// to see what it points at.
type graphExpandRequest struct {
	Type  string `json:"type"`
	ID    int64  `json:"id"`
	Depth int    `json:"depth"`
}

// ExpandGraphNode fetches one specific, previously-collapsed node in
// full and walks its own REF fields, via restEmbedGraphRunner.Expand.
// Returns the same {nodes, edges} shape Graph itself does — the newly
// expanded node (no longer collapsed) plus whatever it points at,
// some of which may itself be collapsed if depth or the fetch ceiling
// ran out again. Merging this into whatever's already on the canvas
// without duplicating anything is the JS side's own job
// (fsm-editor.js's _expandCollapsedNode) — this handler has no notion
// of what's currently rendered client-side at all, only what a fresh
// walk from this one seed discovers.
func (h *queryHandler) ExpandGraphNode(w http.ResponseWriter, r *http.Request) {
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

	var req graphExpandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	depth := req.Depth
	if depth <= 0 {
		depth = 1
	}

	data, err := (restEmbedGraphRunner{}).Expand(r.Context(), c, conn, req.Type, req.ID, depth)
	if err != nil {
		writeJSONError(w, err, name)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

// refFieldInfo is one REF-formatted field on a schema, as the graph
// viewer's own edge-creation flow needs it — see RefFieldsForType.
type refFieldInfo struct {
	Name   string `json:"name"`
	Target string `json:"target,omitempty"` // empty when the target entity type isn't known anywhere (declared or remembered)
}

// RefFieldsForType lists an entity type's own REF-formatted fields and
// their known targets (schema-declared or remembered — the same
// resolution refTargetsForSubmission already does for the entity
// form) — the graph viewer's own client-side answer to "which of this
// node's fields could a new edge be assigned to, and what type of
// node is a valid drop for each." A field with no known target at all
// is still listed (Target omitted) — the JS side treats that as "not
// a valid schema-ful drag candidate" per this session's own agreed
// design (a target that genuinely isn't known anywhere can't be
// validated against a drop, so it isn't offered), rather than
// silently dropping it from the response and losing the distinction
// between "no REF fields at all" and "REF fields exist, just none
// with a known target."
func (h *queryHandler) RefFieldsForType(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entityType := r.PathValue("type")
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

	schema, err := c.GetEntitySchema(r.Context(), entityType)
	fields := []refFieldInfo{}
	hasSchema := err == nil && schema != nil
	if hasSchema {
		targets := refTargetsForSubmission(r.Context(), c, schema)
		for _, f := range schema.Fields {
			if f.Format != "ref" && f.Type != "ref" {
				continue
			}
			fields = append(fields, refFieldInfo{Name: f.Name, Target: targets[f.Name]})
		}
	} else if err != nil && !isNotFoundError(err) {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}
	// A genuinely schema-less type (isNotFoundError, hasSchema false)
	// and a schema-ful type with no REF fields at all (hasSchema true,
	// fields empty) both end up with an empty fields list here, but
	// they mean different things to the JS side's own edge-creation
	// flow: only the former is allowed to fall back to "any target,
	// prompt for a field name" — a schema-ful type's own constraints
	// (whatever fields it does or doesn't declare) are never bypassed
	// just because none of them happen to be usable right now.

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Fields    []refFieldInfo `json:"fields"`
		HasSchema bool           `json:"hasSchema"`
	}{Fields: fields, HasSchema: hasSchema})
}

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
	s.DxpDefName, _ = e.Data["dxp_def_name"].(string)
	s.Resolver, _ = e.Data["resolver"].(string)
	if raw, ok := e.Data["form_fields"]; ok {
		// form_fields arrives as []any (generic JSON decode), not
		// []dxpPresetFormField directly -- round-trip it through
		// json.Marshal/Unmarshal rather than hand-walking the
		// interface{} shape, the same pattern xoluman uses wherever
		// else a nested structure comes back from a generic entity
		// document.
		if b, err := json.Marshal(raw); err == nil {
			_ = json.Unmarshal(b, &s.FormFields)
		}
	}
	return s, true
}
