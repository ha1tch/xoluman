// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strconv"

	mi "github.com/ha1tch/minty"
	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/connstore"
)

// gridPage wraps body in the shared page shell plus what the grid
// editor needs beyond the baseline: Tabulator's stylesheet and script
// (UMD global — window.Tabulator — no import map entry needed for it),
// an import map providing 'lit' (same mechanism as Seam AMS's own
// LitImportMap for its FSM editor), and the grid-editor.js Lit shell as
// a module script.
func gridPage(title, activePath string, body mi.H) mi.H {
	return func(b *mi.Builder) mi.Node {
		extraHead := []mi.Node{
			b.Link(mi.Rel("stylesheet"), mi.Href("/static/vendor/tabulator@6.5.2.min.css")),
			b.Script(mi.Attr("src", "/static/vendor/tabulator@6.5.2.min.js")),
			b.Script(mi.Attr("type", "importmap"), mi.Raw(`{"imports":{"lit":"/static/vendor/lit@3.js"}}`)),
			b.Script(mi.Attr("type", "module"), mi.Attr("src", "/static/js/grid-editor.js")),
		}
		return PageWithHead(title, activePath, extraHead, body)(b)
	}
}

// gridColumn is one Tabulator column definition. Field names match
// Tabulator's documented column-definition config directly (checked
// against the vendored source for the ones that matter for
// correctness — the response/request contract; editor names here are
// standard, long-stable Tabulator features).
type gridColumn struct {
	Title  string `json:"title"`
	Field  string `json:"field"`
	Editor any    `json:"editor"` // string editor name, or false to disable editing on this column
}

// buildGridColumns derives Tabulator column definitions from an entity
// type's schema fields — every non-object/array scalar field, matching
// T-11's design pass (unlike the read-only list's 4-column glanceability
// cap, a grid meant for bulk editing shows everything there is to
// edit). The id column is always first and never editable — entity
// identity, not a value to change.
func buildGridColumns(fields []xclient.FieldDef) []gridColumn {
	cols := []gridColumn{{Title: "ID", Field: "id", Editor: false}}
	for _, f := range fields {
		if f.Type == "object" || f.Type == "array" {
			continue
		}
		editor := "input"
		switch {
		case f.Format == "decimal":
			editor = "input" // stays text — no numeric editor, same reasoning as formengine/RenderFields: avoid float64 precision loss end to end
		case f.Format == "ref" || f.Type == "ref":
			editor = "input" // raw target ID, same v1 limitation as the entity form
		case f.Type == "integer" || f.Type == "number":
			editor = "number"
		case f.Type == "boolean":
			editor = "tickCross"
		}
		cols = append(cols, gridColumn{Title: f.Name, Field: f.Name, Editor: editor})
	}
	return cols
}

// GridView renders the grid editor page for an entity type: the
// Lit-shelled Tabulator component, configured with columns derived from
// the schema and pointed at this entity type's grid-data endpoints.
func (h *EntitiesHandler) GridView(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entityType := r.PathValue("type")

	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	schema, err := c.GetEntitySchema(r.Context(), entityType)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	columns := buildGridColumns(schema.Fields)
	columnsJSON, err := json.Marshal(columns)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	basePath := "/connections/" + name + "/entities/" + entityType
	body := func(b *mi.Builder) mi.Node {
		return b.Div(
			b.Div(mi.Class("flex items-center justify-between mb-4"),
				b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white"), entityType+" — grid view"),
				b.A(mi.Href(basePath), mi.Class(btnSecondary), "Back to list"),
			),
			b.Script(mi.Attr("type", "application/json"), mi.ID("grid-columns"), mi.Raw(string(columnsJSON))),
			// minty has no generic custom-element Builder method (only
			// createElement, unexported) — mi.Raw is the same escape
			// hatch Seam AMS's own FSM editor page uses for its
			// <seam-fsm-editor> tag. Values here are escaped with
			// html.EscapeString before interpolating, which Seam's own
			// version does not do for its own attribute values — worth
			// doing regardless of how constrained connection/entity-type
			// names happen to be in practice.
			mi.Raw(fmt.Sprintf(
				`<xolu-grid-editor data-url="%s" save-url="%s"></xolu-grid-editor>`,
				html.EscapeString(basePath+"/grid-data"),
				html.EscapeString(basePath+"/grid-data"),
			)),
		)
	}

	WriteHTML(w, gridPage(entityType+" — grid", r.URL.Path, body))
}

// gridPageSize is the default page size requested from Tabulator's
// remote-pagination mode when the client doesn't specify one.
const gridPageSize = 50

// gridDataResponse matches Tabulator's confirmed default remote-
// pagination response shape exactly (checked directly against the
// vendored tabulator@6.5.2.min.js source, not assumed from docs):
// {data, last_page}, with last_row as an optional precise total-row
// count Tabulator uses instead of estimating from last_page*pageSize.
type gridDataResponse struct {
	Data     []map[string]any `json:"data"`
	LastPage int              `json:"last_page"`
	LastRow  int              `json:"last_row"`
}

// GridData serves GET .../grid-data — the JSON endpoint Tabulator's
// ajaxURL + pagination:"remote" consumes directly. Page/size query
// params are named to match what the grid's dataSendParams config is
// set to send (page/per_page), which was chosen explicitly to match
// xolu's own List() convention rather than relying on Tabulator's
// internal (and, from the vendored source, not confidently
// determinable) default param names.
func (h *EntitiesHandler) GridData(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entityType := r.PathValue("type")

	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		writeJSONError(w, err, name)
		return
	}

	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	perPage := gridPageSize
	if s, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && s > 0 {
		perPage = s
	}

	result, err := c.List(r.Context(), entityType, &xclient.ListParams{
		Limit:  perPage,
		Offset: (page - 1) * perPage,
	})
	if err != nil {
		writeJSONError(w, err, name)
		return
	}

	resp := gridDataResponse{
		Data:     make([]map[string]any, len(result.Entities)),
		LastPage: result.TotalPages,
		LastRow:  result.TotalItems,
	}
	if resp.LastPage < 1 {
		resp.LastPage = 1
	}
	for i, e := range result.Entities {
		doc := e.Data
		if doc == nil {
			doc = map[string]any{}
		}
		doc["id"] = e.ID // ensure the row identity Tabulator/the save step needs is always present, even if the entity document itself omits it
		resp.Data[i] = doc
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// gridSaveRequest is one row's worth of edited cells, keyed by field
// name — deliberately not the full document, so the save step can only
// ever Patch (partial update), never Update (full replace). Sending a
// full-document Update here would silently drop any field the grid
// doesn't show as a column, exactly the data-loss trap T-11's design
// pass exists to prevent.
type gridSaveRequest struct {
	ID      int64          `json:"id"`
	Changes map[string]any `json:"changes"`
}

// gridSaveResult reports one row's save outcome.
type gridSaveResult struct {
	ID      int64  `json:"id"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// GridSave serves POST .../grid-data — a batch of per-row partial
// changes, each applied independently via Client.Patch. One row's
// failure doesn't affect another's, same philosophy as import (T-05):
// the response reports per-row results rather than an all-or-nothing
// outcome.
func (h *EntitiesHandler) GridSave(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entityType := r.PathValue("type")

	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		writeJSONError(w, err, name)
		return
	}

	var rows []gridSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&rows); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	results := make([]gridSaveResult, len(rows))
	for i, row := range rows {
		if _, err := c.Patch(r.Context(), entityType, row.ID, row.Changes); err != nil {
			results[i] = gridSaveResult{ID: row.ID, Success: false, Message: err.Error()}
			continue
		}
		results[i] = gridSaveResult{ID: row.ID, Success: true}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

// writeJSONError writes a JSON-shaped error for the grid's AJAX
// endpoints — these are consumed by Tabulator/fetch, not rendered as
// an HTML page, so writeUpstreamError's page-shell error response
// would be the wrong content type entirely.
func writeJSONError(w http.ResponseWriter, err error, connName string) {
	status := http.StatusBadGateway
	if errors.Is(err, connstore.ErrNotFound) {
		status = http.StatusNotFound
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
