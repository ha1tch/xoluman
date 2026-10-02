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
	"strings"

	mi "github.com/ha1tch/minty"
	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/connstore"
	"github.com/ha1tch/xoluman/internal/fieldmeta"
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
			// Loaded after the base stylesheet so its .dark-scoped
			// rules win on specificity ties without needing !important
			// beyond what the base theme itself already forces. See
			// the file's own doc comment for why this is a hand-
			// written override rather than swapping in Tabulator's own
			// alternate "midnight" theme.
			b.Link(mi.Rel("stylesheet"), mi.Href("/static/css/tabulator-dark-overrides.css")),
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
	Title        string `json:"title"`
	Field        string `json:"field"`
	Editor       any    `json:"editor"` // string editor name, or false to disable editing on this column
	EditorParams any    `json:"editorParams,omitempty"`
}

// listEditorParams is the "list" editor's config shape for its
// dropdown choices — a plain {value: label} object, confirmed against
// the vendored Tabulator source directly (not assumed from docs) when
// T-15 first scoped this. Kept as its own type rather than an inline
// map literal so buildGridColumns' intent (a select field, not a free
// text input) is visible at the call site.
type listEditorParams struct {
	Values map[string]string `json:"values"`
}

// buildGridColumns derives Tabulator column definitions from an entity
// type's schema fields — every non-object/array scalar field, matching
// T-11's design pass (unlike the read-only list's 4-column glanceability
// cap, a grid meant for bulk editing shows everything there is to
// edit). The id column is always first and never editable — entity
// identity, not a value to change.
//
// fieldOptions carries any xoluman_field_meta dropdown configuration
// for this entity type (same source resolveFormOptions already uses
// for the entity form's own select fields) — a configured field gets
// Tabulator's "list" editor instead of a bare text input, matching
// the entity form's own dropdown for the identical field rather than
// letting the grid be the one place that still shows a raw value.
func buildGridColumns(fields []xclient.FieldDef, fieldOptions map[string][]fieldmeta.Option) []gridColumn {
	cols := []gridColumn{{Title: "ID", Field: "id", Editor: false}}
	for _, f := range fields {
		if f.Type == "object" || f.Type == "array" {
			continue
		}
		if opts, ok := fieldOptions[f.Name]; ok && len(opts) > 0 {
			values := make(map[string]string, len(opts))
			for _, o := range opts {
				values[o.Key] = o.Value
			}
			cols = append(cols, gridColumn{
				Title: f.Name, Field: f.Name, Editor: "list",
				EditorParams: listEditorParams{Values: values},
			})
			continue
		}
		editor := "input"
		switch {
		case f.Format == "decimal":
			editor = "input" // stays text — no numeric editor, same reasoning as formengine/RenderFields: avoid float64 precision loss end to end
		case f.Format == "ref" || f.Type == "ref":
			editor = "input" // raw target ID typed in; GridSave reconstructs the real {type:REF,...} shape server-side (reconstructRefValues) before writing — no picker UI here yet, that's the remaining v1 limitation, not save failing outright
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

	// Same xoluman_field_meta lookup resolveFormOptions already does
	// for the entity form's own select fields — best-effort, a lookup
	// failure just means every field renders as a plain text input in
	// the grid, same as before this existed, not a page-level error.
	fieldOptions := map[string][]fieldmeta.Option{}
	if metas, err := fieldmeta.LoadForEntityType(r.Context(), c, schema.Name); err == nil {
		for fieldName, m := range metas {
			if resolved, err := fieldmeta.ResolveOptions(r.Context(), c, m); err == nil {
				fieldOptions[fieldName] = resolved
			}
		}
	}

	columns := buildGridColumns(schema.Fields, fieldOptions)
	columnsJSON, err := json.Marshal(columns)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	basePath := entitiesBasePath(name, entityType)
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

	// The grid's own cell editor has no way to submit anything but a
	// raw string/number per cell (Tabulator's "input"/"number"/
	// "tickCross" editors, see buildGridColumns) — a REF-typed column
	// edits as a bare target ID, exactly like the entity form's own
	// text input did before refTargetsForSubmission existed. Without
	// this same reconstruction step here, xolu's own validation
	// correctly rejects the bare ID outright (confirmed directly:
	// XOLU-VL001 on every attempt) — not silent corruption, since
	// xolu's write-side validation catches it, but the grid's own
	// REF-column editing was completely unusable as a result. Same
	// fix, same helper, applied to the one write path that hadn't
	// gotten it yet.
	schema, schemaErr := c.GetEntitySchema(r.Context(), entityType)
	if schemaErr != nil && !isNotFoundError(schemaErr) {
		writeJSONError(w, schemaErr, name)
		return
	}
	refTargets := refTargetsForSubmission(r.Context(), c, schema)

	// refFieldNames identifies every REF-formatted field even when its
	// target isn't known yet — needed so reconstructRefValues can
	// still recognize a REF field and accept the "entityType:id"
	// compound form for it (see reconstructRefValues' own doc comment)
	// rather than only working for fields refTargets already covers.
	// Most of this session's own CRM schema declares `{"format":"ref"}`
	// with no target at all (confirmed directly against the real
	// schema, not assumed) — refTargets alone leaves the grid's REF
	// columns for exactly those fields unusable, which the schema-
	// declared/remembered-target path above doesn't reach.
	refFieldNames := make(map[string]bool)
	if schema != nil {
		for _, f := range schema.Fields {
			if f.Format == "ref" || f.Type == "ref" {
				refFieldNames[f.Name] = true
			}
		}
	}

	results := make([]gridSaveResult, len(rows))
	for i, row := range rows {
		changes, newlyLearned, convErr := reconstructRefValues(row.Changes, refTargets, refFieldNames)
		if convErr != nil {
			results[i] = gridSaveResult{ID: row.ID, Success: false, Message: convErr.Error()}
			continue
		}
		if _, err := c.Patch(r.Context(), entityType, row.ID, changes); err != nil {
			results[i] = gridSaveResult{ID: row.ID, Success: false, Message: err.Error()}
			continue
		}
		results[i] = gridSaveResult{ID: row.ID, Success: true}
		// Best-effort, same as the entity form's own remember step —
		// a failure here just means the next grid edit to this field
		// needs the compound form again too, not a reason to fail a
		// save that already genuinely succeeded.
		for field, target := range newlyLearned {
			_ = fieldmeta.RememberRefTarget(r.Context(), c, entityType, field, target)
			refTargets[field] = target // so later rows in this same batch benefit immediately too
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

// reconstructRefValues rewrites any changed field that's a known REF
// field from the grid's own raw submitted value into the structured
// {"type":"REF","entity":target,"id":N} shape xolu's write path
// requires — the identical shape formengine.ParseFormValues already
// builds for the entity form, applied here to the grid's differently-
// shaped input (a decoded JSON map of raw values, not a url.Values
// form submission) rather than routed through ParseFormValues itself,
// which expects the latter. Fields not in refFieldNames pass through
// completely unchanged — this never touches a non-REF field's value.
//
// Two cases for a recognized REF field:
//   - refTargets already knows the target (schema-declared or
//     previously remembered): the raw value is a plain ID.
//   - refTargets doesn't know it (confirmed the common case for this
//     session's own CRM schema — most REF fields declare
//     {"format":"ref"} with no "target" at all): the grid's single
//     text-input cell editor has no companion field the way the
//     entity form does, so the raw value is expected as the compound
//     "entityType:id" form instead (e.g. "users:5"). Returned via
//     newlyLearned so the caller can remember it (fieldmeta.
//     RememberRefTarget) for every future edit to this field —
//     exactly the entity form's own behavior, just triggered from a
//     different input shape.
func reconstructRefValues(changes map[string]any, refTargets map[string]string, refFieldNames map[string]bool) (out map[string]any, newlyLearned map[string]string, err error) {
	if len(refFieldNames) == 0 {
		return changes, nil, nil
	}
	out = make(map[string]any, len(changes))
	for field, raw := range changes {
		if !refFieldNames[field] {
			out[field] = raw
			continue
		}
		if target, known := refTargets[field]; known {
			id, convErr := coerceToInt64(raw)
			if convErr != nil {
				return nil, nil, fmt.Errorf("%s: must be a valid ID, got %v", field, raw)
			}
			out[field] = map[string]any{"type": "REF", "entity": target, "id": id}
			continue
		}
		// Target unknown — require the compound form.
		s, ok := raw.(string)
		target, idStr, found := "", "", false
		if ok {
			target, idStr, found = strings.Cut(s, ":")
		}
		if !found || target == "" || idStr == "" {
			return nil, nil, fmt.Errorf(
				"%s: target entity type unknown for this field — type \"entityType:id\" (e.g. \"users:5\") once, and every later edit to this field only needs the plain ID", field)
		}
		id, convErr := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
		if convErr != nil {
			return nil, nil, fmt.Errorf("%s: %q is not a valid ID", field, idStr)
		}
		out[field] = map[string]any{"type": "REF", "entity": target, "id": id}
		if newlyLearned == nil {
			newlyLearned = map[string]string{}
		}
		newlyLearned[field] = target
	}
	return out, newlyLearned, nil
}

// coerceToInt64 accepts either shape Tabulator could plausibly send
// for a cell whose editor is a plain text input: a JSON number
// (float64, since encoding/json decodes all JSON numbers that way) or
// a numeric string.
func coerceToInt64(v any) (int64, error) {
	switch t := v.(type) {
	case float64:
		return int64(t), nil
	case string:
		return strconv.ParseInt(strings.TrimSpace(t), 10, 64)
	default:
		return 0, fmt.Errorf("unsupported value type %T", v)
	}
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
