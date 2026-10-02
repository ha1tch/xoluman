// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	mi "github.com/ha1tch/minty"
	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/connstore"
	"github.com/ha1tch/xoluman/internal/fieldmeta"
	"github.com/ha1tch/xoluman/internal/formengine"
	"github.com/ha1tch/xoluman/internal/importer"
	"github.com/ha1tch/xoluman/internal/modules"
	"github.com/ha1tch/xoluman/internal/xoluext"
)

// entitiesPerPage is fixed for now — a per-connection or per-request
// page-size preference is a real feature, not a stopgap, and isn't
// needed yet.
const entitiesPerPage = 25

// RegisterEntitiesModule registers the Entities module's routes onto
// reg, backed by store. No top-level nav entry (Label left empty,
// which Page() treats as "routes only") — entities are reached
// contextually per-connection, via the "Entities" link on each
// connection row, not a standalone always-visible nav item.
func RegisterEntitiesModule(reg *modules.Registry, store connstore.Store) {
	entities := NewEntitiesHandler(store)
	reg.Register(modules.Module{
		ID: "entities",
		MountRoutes: func(mux *http.ServeMux) {
			mux.HandleFunc("GET /connections/{name}/entities", entities.List)
			mux.HandleFunc("GET /connections/{name}/entities/{type}", entities.Show)
			mux.HandleFunc("GET /connections/{name}/entities/{type}/new", entities.NewForm)
			mux.HandleFunc("POST /connections/{name}/entities/{type}", entities.Create)
			mux.HandleFunc("GET /connections/{name}/entities/{type}/import", entities.ImportForm)
			mux.HandleFunc("POST /connections/{name}/entities/{type}/import", entities.ImportPreview)
			mux.HandleFunc("POST /connections/{name}/entities/{type}/import/{id}/confirm", entities.ImportConfirm)
			mux.HandleFunc("GET /connections/{name}/entities/{type}/{id}/edit", entities.EditForm)
			mux.HandleFunc("POST /connections/{name}/entities/{type}/{id}", entities.Update)
			mux.HandleFunc("GET /connections/{name}/entities/{type}/{id}/delete-confirm", entities.DeleteConfirm)
			mux.HandleFunc("POST /connections/{name}/entities/{type}/{id}/delete", entities.Delete)
			mux.HandleFunc("GET /connections/{name}/entities/{type}/grid", entities.GridView)
			mux.HandleFunc("GET /connections/{name}/entities/{type}/grid-data", entities.GridData)
			mux.HandleFunc("POST /connections/{name}/entities/{type}/grid-data", entities.GridSave)
			mux.HandleFunc("GET /connections/{name}/entities/{type}/promote", entities.PromotePreview)
			mux.HandleFunc("POST /connections/{name}/entities/{type}/promote", entities.Promote)
		},
	})
}

// EntitiesHandler serves every /connections/{name}/entities... route.
// Unlike ConnectionsHandler, every request here first resolves which
// xolu instance it's talking to from the {name} path value — there is
// no single "the" client, xoluman manages several.
type EntitiesHandler struct {
	store    connstore.Store
	sessions *importer.SessionStore
}

// NewEntitiesHandler returns an EntitiesHandler backed by store.
func NewEntitiesHandler(store connstore.Store) *EntitiesHandler {
	return &EntitiesHandler{store: store, sessions: importer.NewSessionStore()}
}

// clientFor resolves the named connection and builds a client for it.
// Every handler in this file starts here.
func (h *EntitiesHandler) clientFor(ctx context.Context, name string) (*xclient.Client, error) {
	conn, err := h.store.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	return xoluext.BuildClient(conn), nil
}

// writeConnectionNotFound renders a plain 404 when {name} doesn't match
// a stored connection — every handler below hits this the same way.
func writeConnectionNotFound(w http.ResponseWriter, name string) {
	http.Error(w, fmt.Sprintf("no connection named %q", name), http.StatusNotFound)
}

// writeUpstreamError renders a general error page when the xolu
// instance itself returns an error (unreachable, 4xx/5xx) — distinct
// from writeConnectionNotFound, which means xoluman doesn't know this
// connection at all.
func writeUpstreamError(w http.ResponseWriter, connName string, activePath string, err error) {
	body := func(b *mi.Builder) mi.Node {
		return b.Div(
			b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white"), "Something went wrong"),
			b.P(mi.Class("text-red-600 dark:text-red-400 mt-2"), err.Error()),
			b.A(mi.Href("/connections"), mi.Class(btnSecondary), mi.Style("display:inline-flex; margin-top:1rem;"), "Back to connections"),
		)
	}
	w.WriteHeader(http.StatusBadGateway)
	WriteHTML(w, Page("Error", activePath, body))
}

// ─── Entity type list ────────────────────────────────────────────────────

// List renders the entity types available on the named connection.
// entitiesBasePath returns "/connections/{name}/entities/{entityType}"
// with both segments properly escaped via url.PathEscape — not the
// raw values. A real, confirmed bug this fixes: connection names and
// entity type names flowed unescaped into every URL path built from
// them throughout this codebase. A connection named "My Server"
// rendered a literal, unescaped space directly into href values; any
// character with real meaning in a URL path (a slash, a percent sign,
// a hash) could shift what the server-side router actually captures
// for adjacent path segments — plausibly the root cause of "clicking
// any entity gives XOLU-ST004: Invalid ID," though that couldn't be
// conclusively reproduced before this fix; this closes the class of
// bug regardless.
func entitiesBasePath(connName, entityType string) string {
	return "/connections/" + url.PathEscape(connName) + "/entities/" + url.PathEscape(entityType)
}

func (h *EntitiesHandler) List(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// Jump-by-name kept as a fallback for typing an exact known name
	// directly — marginally faster than scanning a long list — but
	// discovery itself now uses ListEntities (xolu v0.25.0), the real
	// fix for what this was originally a workaround for: it lists
	// every entity type with actual data, schemaless or not, unlike
	// ListEntityTypes (confirmed: reflects validator.LoadedEntities(),
	// registered schemas only — nothing to do with what data exists).
	if jump := r.URL.Query().Get("type"); jump != "" {
		http.Redirect(w, r, entitiesBasePath(name, jump), http.StatusSeeOther)
		return
	}

	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	entries, err := c.ListEntities(r.Context(), false)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	body := func(b *mi.Builder) mi.Node {
		header := b.Div(mi.Class("mb-4"),
			b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white"), "Entities on "+name),
		)
		jumpForm := b.Form(mi.Attr("method", "get"), mi.Attr("action", "/connections/"+url.PathEscape(name)+"/entities"), mi.Class("mb-4 flex gap-2 items-end"),
			b.Div(
				b.Label(mi.For("type"), mi.Class("block mb-1 text-sm text-gray-600 dark:text-gray-400"), "Jump to a type by name"),
				b.Input(mi.Type("text"), mi.ID("type"), mi.Name("type"), mi.Placeholder("e.g. gadgets"), mi.Class(formInputClass)),
			),
			b.Button(mi.Type("submit"), mi.Class(btnSecondary), "Go"),
		)
		if len(entries) == 0 {
			return b.Div(header, jumpForm,
				b.Div(mi.Class("text-gray-500 dark:text-gray-400 text-sm"), "No entity types have any data on this instance yet."),
			)
		}
		rows := make([]mi.Node, len(entries))
		for i, e := range entries {
			schemaLabel := "no schema"
			schemaClass := "text-gray-400 dark:text-gray-500"
			var promoteLink interface{}
			if e.HasSchema {
				schemaLabel = "has schema"
				schemaClass = "text-gray-600 dark:text-gray-400"
			} else {
				promoteLink = b.A(mi.Href(entitiesBasePath(name, e.EntityType)+"/promote"),
					mi.Class("text-indigo-600 dark:text-indigo-400 hover:underline text-xs"), "Promote to schema")
			}
			rows[i] = b.Tr(
				b.Td(mi.Class("px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm"),
					b.A(mi.Href(entitiesBasePath(name, e.EntityType)), mi.Class("text-indigo-600 dark:text-indigo-400 hover:underline"), e.EntityType),
				),
				b.Td(mi.Class("px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm text-gray-600 dark:text-gray-400"), strconv.FormatInt(e.Count, 10)),
				b.Td(mi.Class("px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm "+schemaClass), schemaLabel),
				b.Td(mi.Class("px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm"), promoteLink),
			)
		}
		table := Table([]string{"Entity type", "Rows", "Schema", ""}, rows, "")(b)
		return b.Div(header, jumpForm, table)
	}

	WriteHTML(w, Page("Entities", r.URL.Path, body))
}

// ─── Entity list (paginated) ────────────────────────────────────────────

func (h *EntitiesHandler) Show(w http.ResponseWriter, r *http.Request) {
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

	// Schemas are not obligatory in xolu — GetEntitySchema 404s for a
	// perfectly real entity type that simply has no schema registered
	// (confirmed directly: POST-then-GET on an unregistered type works
	// fine end to end; only /api/v1/schema/{entity} 404s). That must
	// not fail this page — c.List below doesn't need a schema at all,
	// only the preview-column choice does, and that has a fallback.
	var previewCols []xclient.FieldDef
	schema, err := c.GetEntitySchema(r.Context(), entityType)
	switch {
	case err == nil:
		previewCols = previewFields(schema.Fields)
	case isNotFoundError(err):
		// Resolved after fetching below, from the data itself.
	default:
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}

	result, err := c.List(r.Context(), entityType, &xclient.ListParams{
		Limit:  entitiesPerPage,
		Offset: (page - 1) * entitiesPerPage,
	})
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	if previewCols == nil {
		previewCols = previewFields(inferFieldsFromEntities(result.Entities))
	}

	basePath := entitiesBasePath(name, entityType)
	cfg := ListPageConfig{Title: entityType, CreateLabel: "New " + entityType, CreateURL: basePath + "/new"}
	refTargets := refTargetsForSubmission(r.Context(), c, schema) // nil-schema case (inferred fields) still returns nil — inference has no way to know which fields are references
	body := func(b *mi.Builder) mi.Node {
		importLink := b.A(mi.Href(basePath+"/import"), mi.Class(btnSecondary+" mb-3 inline-flex"), "Import from file")
		gridLink := b.A(mi.Href(basePath+"/grid"), mi.Class(btnSecondary+" mb-3 inline-flex"), "Grid view")
		links := b.Div(mi.Class("flex gap-2"), importLink, gridLink)
		return b.Div(links, ListPage(cfg, entityListTable(name, entityType, previewCols, refTargets, result))(b))
	}

	WriteHTML(w, Page(entityType, r.URL.Path, body))
}

// maxPreviewColumns caps how many of an entity type's own fields show
// up as columns in the list table — a generic browser across arbitrary
// schemas needs a bound, or a wide schema makes an unusable table.
// Full field access is what the edit form (formengine) is for.
const maxPreviewColumns = 4

// previewFields picks which schema fields are worth showing as list
// columns: object/array values don't render sensibly inline (that's
// what the edit form's JSON textarea is for), and the count is capped
// at maxPreviewColumns regardless of schema size.
func previewFields(fields []xclient.FieldDef) []xclient.FieldDef {
	out := make([]xclient.FieldDef, 0, maxPreviewColumns)
	for _, f := range fields {
		// A REF field's own JSON Schema Type is "object" (the
		// reference-ness lives in Format, a separate key — confirmed
		// directly against a real schema: {"type":"object",
		// "format":"ref"}) — excluding every object-typed field here
		// unconditionally silently excluded every REF field too, so
		// the list row renderer's own, already-correct REF handling
		// (RefJumpButton, a few lines below in this file) never once
		// got a REF field to render in practice. Genuine object/array
		// fields (no meaningful single-line glance value) still stay
		// excluded; only the ref-formatted subset of "object" is let
		// through.
		if (f.Type == "object" && f.Format != "ref") || f.Type == "array" {
			continue
		}
		out = append(out, f)
		if len(out) == maxPreviewColumns {
			break
		}
	}
	return out
}

// isNotFoundError reports whether err is xolu's own 404 — used to
// distinguish "this specific thing doesn't exist" (a fine, expected
// condition to degrade gracefully from — e.g. no schema registered)
// from any other failure (connectivity, auth, a genuine server error),
// which should still surface as an error rather than being silently
// swallowed.
func isNotFoundError(err error) bool {
	xoluErr, ok := err.(*xclient.Error)
	return ok && xoluErr.HTTPStatus == 404
}

// inferFieldsFromEntities derives a basic field list from actually-
// fetched documents when no schema is registered for the entity type —
// schemas are not obligatory in xolu, and entities created without one
// are otherwise invisible to every schema-dependent page. This is a
// best-effort fallback, not a real schema: type is inferred from each
// key's JSON value shape (the same string/float64/bool/object/array
// dispatch formengine's own stringifyValue already uses), no field is
// ever marked Required (there's no way to know that without a real
// schema), and the field set is the union across every fetched row —
// a single row might not show every field the type actually has.
func inferFieldsFromEntities(entities []xclient.Entity) []xclient.FieldDef {
	seen := make(map[string]bool)
	var fields []xclient.FieldDef
	for _, e := range entities {
		for key, val := range e.Data {
			if key == "id" || key == "_version" || seen[key] {
				continue
			}
			seen[key] = true
			fields = append(fields, xclient.FieldDef{Name: key, Type: inferJSONType(val)})
		}
	}
	return fields
}

// inferJSONType maps a decoded-JSON value (encoding/json's default
// unmarshal-into-any shapes) to the closest JSON Schema type name.
func inferJSONType(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		return "string"
	}
}

// refTargetsByField builds a field-name -> target-entity-type map from
// schema.Refs, for the list preview's cheap ref-link rendering — no
// label resolution here (that would mean one extra Get per ref cell
// per row, an N+1 problem this view deliberately avoids; the edit
// form's resolveFormOptions already does resolve labels, where the
// cost is bounded to a handful of fields on one entity, not one per
// row across a whole page). Returns nil when schema is nil (the
// schema-less/inferred-fields case) — inference has no way to know
// which fields are references.
func refTargetsByField(schema *xclient.EntitySchema) map[string]string {
	if schema == nil {
		return nil
	}
	targets := make(map[string]string, len(schema.Refs))
	for _, ref := range schema.Refs {
		if ref.Target != "" {
			targets[ref.Name] = ref.Target
		}
	}
	return targets
}

// refTargetsForSubmission is refTargetsByField plus any remembered
// targets (fieldmeta.RememberRefTarget) — required specifically at
// ParseFormValues call sites, not just the rendering path. The reason
// this can't just be refTargetsByField: once a target is remembered,
// resolveFormOptions's own RefTargets knows about it and refInput
// correctly stops rendering the companion f.Name+"__ref_entity" input
// for that field (it's not needed anymore) — but if ParseFormValues is
// then called with only the schema-derived map, it has no way to know
// the target either, the companion field genuinely isn't in the
// submitted form, and it falls back to a bare number that xolu
// rejects. This mirrors resolveFormOptions's own remembered-target
// lookup so the two stay in agreement about what's "known."
func refTargetsForSubmission(ctx context.Context, c *xclient.Client, schema *xclient.EntitySchema) map[string]string {
	targets := refTargetsByField(schema)
	if schema == nil {
		return targets
	}
	if targets == nil {
		targets = map[string]string{}
	}
	metas, err := fieldmeta.LoadForEntityType(ctx, c, schema.Name)
	if err != nil {
		return targets // best-effort — a lookup failure just means no remembered targets apply this time, not a hard failure
	}
	for _, ref := range schema.Refs {
		if _, known := targets[ref.Name]; known {
			continue
		}
		if target, ok := fieldmeta.LookupRememberedTarget(metas, ref.Name); ok {
			targets[ref.Name] = target
		}
	}
	return targets
}

// rememberNewRefTargets saves any ref field's companion
// f.Name+"__ref_entity" value that differs from what's already known
// (already contains the schema-declared and previously-remembered
// targets — see refTargetsForSubmission) — called only after a
// successful create/update, so a target that turned out to be wrong
// (the save itself failed) is never remembered. Best-effort: a save
// failure here doesn't affect the entity save that already succeeded,
// just means this specific field won't auto-fill next time.
func rememberNewRefTargets(ctx context.Context, c *xclient.Client, entityType string, schema *xclient.EntitySchema, form url.Values, alreadyKnown map[string]string) {
	if schema == nil {
		return
	}
	for _, ref := range schema.Refs {
		if ref.Target != "" {
			continue // schema already declares it — nothing to remember
		}
		supplied := form.Get(ref.Name + "__ref_entity")
		if supplied == "" || supplied == alreadyKnown[ref.Name] {
			continue
		}
		_ = fieldmeta.RememberRefTarget(ctx, c, entityType, ref.Name, supplied)
	}
}

func entityListTable(connName, entityType string, preview []xclient.FieldDef, refTargets map[string]string, result *xclient.ListResult) mi.H {
	return func(b *mi.Builder) mi.Node {
		if len(result.Entities) == 0 {
			return b.Div(mi.Class("text-center py-12 text-gray-500 dark:text-gray-400"), "No "+entityType+" entities yet.")
		}

		basePath := entitiesBasePath(connName, entityType)
		td := "px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm text-gray-700 dark:text-gray-300"

		columns := make([]string, 0, len(preview)+3)
		columns = append(columns, "ID")
		for _, f := range preview {
			columns = append(columns, f.Name)
		}
		columns = append(columns, "", "")

		rows := make([]mi.Node, len(result.Entities))
		for i, e := range result.Entities {
			idStr := strconv.FormatInt(e.ID, 10)
			editURL := basePath + "/" + idStr + "/edit"
			deleteConfirmURL := basePath + "/" + idStr + "/delete-confirm"

			cells := make([]interface{}, 0, len(preview)+3)
			cells = append(cells, b.Td(mi.Class(td), idStr))
			for _, f := range preview {
				value := e.Data[f.Name]
				if f.Format == "ref" || f.Type == "ref" {
					if id, embeddedLabel, ok := refValueInfo(value); ok {
						linkText := embeddedLabel
						if linkText == "" {
							linkText = strconv.FormatInt(id, 10)
						}
						if target, known := refTargets[f.Name]; known {
							refURL := entitiesBasePath(connName, target) + "/" + strconv.FormatInt(id, 10) + "/edit"
							cells = append(cells, b.Td(mi.Class(td),
								b.A(mi.Href(refURL), mi.Class("text-indigo-600 dark:text-indigo-400 hover:underline"), linkText),
								RefJumpButton(refURL)(b),
							))
						} else {
							// Target entity type genuinely isn't known
							// anywhere (no schema declaration, nothing
							// remembered yet) — still show the readable
							// id/embedded-label form rather than
							// falling through to previewValue, whose
							// default case has no real handling for a
							// raw map and previously leaked Go's own
							// %v formatting straight into the page
							// (confirmed directly: a cell literally
							// read "map[entity:companies id:13
							// type:REF]"). No clickable link here,
							// since building one needs the target
							// entity type this field doesn't have.
							cells = append(cells, b.Td(mi.Class(td), linkText))
						}
						continue
					}
				}
				cells = append(cells, b.Td(mi.Class(td), previewValue(value)))
			}
			cells = append(cells,
				b.Td(mi.Class(td), ModalTriggerButton("Edit", "Edit "+entityType, editURL, "text-indigo-600 dark:text-indigo-400 hover:underline bg-transparent border-0 p-0 cursor-pointer text-sm")(b)),
				b.Td(mi.Class(td), ModalTriggerButton("Delete", "Delete "+entityType, deleteConfirmURL, btnDanger)(b)),
			)
			rows[i] = b.Tr(cells...)
		}

		table := Table(columns, rows, "")(b)

		var pager interface{}
		if result.TotalPages > 1 {
			pager = paginationBar(basePath, result.Page, result.TotalPages)(b)
		}

		return b.Div(table, pager)
	}
}

// previewMaxStringLen bounds how much of a string field shows in a list
// cell — the edit form is where the full value lives, this is a glance,
// not a data dump.
const previewMaxStringLen = 60

// previewValue renders a single list-cell value from an entity's decoded
// JSON data (string, float64, bool, or nil/absent). Genuine object/array
// fields never reach here — previewFields excludes them. REF-formatted
// fields (JSON Schema type "object", format "ref") also never reach
// here: the row-rendering loop above intercepts them before this
// function is called at all, rendering a RefJumpButton instead.
func previewValue(v any) string {
	switch t := v.(type) {
	case nil:
		return "—"
	case string:
		if len(t) > previewMaxStringLen {
			return t[:previewMaxStringLen-1] + "…"
		}
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func paginationBar(basePath string, page, totalPages int) mi.H {
	return func(b *mi.Builder) mi.Node {
		links := make([]interface{}, 0, totalPages)
		for p := 1; p <= totalPages; p++ {
			class := "px-2 py-1 text-sm text-gray-600 dark:text-gray-400 hover:underline"
			if p == page {
				class = "px-2 py-1 text-sm font-semibold text-gray-900 dark:text-white"
			}
			links = append(links, b.A(mi.Href(basePath+"?page="+strconv.Itoa(p)), mi.Class(class), strconv.Itoa(p)))
		}
		return b.Div(append([]interface{}{mi.Class("flex gap-1 mt-4")}, links...)...)
	}
}

// ─── Create ──────────────────────────────────────────────────────────────

// NewForm renders the create form as a full page — entity schemas can
// have many fields, unlike the connections form, so this deliberately
// doesn't reuse the modal pattern the way "New connection" does.
// resolveEntityFields returns the field list to render/parse a form
// against — from a real registered schema when one exists, or inferred
// from actual data when none is (schemas are not obligatory in xolu;
// see docs/KNOWN_ISSUES.md's recorded finding). known, when non-nil, is
// used directly to infer from without an extra round trip (EditForm/
// Update already have the entity in hand); otherwise up to one
// existing row is fetched via List (NewForm/Create, which have no
// single entity yet).
//
// schemaOut is non-nil only when a real schema was found — passed to
// resolveFormOptions for ref-link resolution, which an inferred field
// list has no basis for (inference can't know what's a reference).
//
// ok is false only when there is truly nothing to build a form from:
// no schema AND no existing data to infer from either. The caller
// should show a clear message then, not attempt to render a
// meaningless empty form.
func resolveEntityFields(ctx context.Context, c *xclient.Client, entityType string, known *xclient.Entity) (fields []xclient.FieldDef, schemaOut *xclient.EntitySchema, ok bool, err error) {
	schema, schemaErr := c.GetEntitySchema(ctx, entityType)
	switch {
	case schemaErr == nil:
		return schema.Fields, schema, true, nil
	case !isNotFoundError(schemaErr):
		return nil, nil, false, schemaErr
	}

	if known != nil {
		return inferFieldsFromEntities([]xclient.Entity{*known}), nil, true, nil
	}

	result, err := c.List(ctx, entityType, &xclient.ListParams{Limit: 1})
	if err != nil {
		return nil, nil, false, err
	}
	if len(result.Entities) == 0 {
		return nil, nil, false, nil // truly nothing to infer from — not an error, a real "nothing to show yet" state
	}
	return inferFieldsFromEntities(result.Entities), nil, true, nil
}

// noFieldsToInferBody renders the message shown when an entity type has
// neither a registered schema nor any existing data — there is nothing
// for the generic form to infer fields from, and pretending otherwise
// with an empty form would be worse than saying so plainly.
func noFieldsToInferBody(entityType string) mi.H {
	return func(b *mi.Builder) mi.Node {
		return b.Div(
			b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white mb-2"), "Can't build a form for "+entityType),
			b.P(mi.Class("text-gray-600 dark:text-gray-400 text-sm"),
				entityType+" has no registered schema and no existing rows to infer fields from — there's nothing for the generic form to work with yet. "+
					"Create the first record directly via the API, or register a schema, then this form will work."),
		)
	}
}

func (h *EntitiesHandler) NewForm(w http.ResponseWriter, r *http.Request) {
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

	fields, schema, ok, err := resolveEntityFields(r.Context(), c, entityType, nil)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}
	if !ok {
		WriteModalAware(w, r, "New "+entityType, noFieldsToInferBody(entityType))
		return
	}

	basePath := entitiesBasePath(name, entityType)
	opts := resolveFormOptionsMaybeSchema(r.Context(), c, name, entityType, schema, nil, nil)
	body := entityFormBody("New "+entityType, basePath, fields, opts, "")
	WriteModalAware(w, r, "New "+entityType, body)
}

// Create parses and validates the submitted form, saves on success, and
// redisplays the form with inline errors on failure — never losing what
// was already typed.
func (h *EntitiesHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	fields, schema, ok, err := resolveEntityFields(r.Context(), c, entityType, nil)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}
	if !ok {
		WriteModalAware(w, r, "New "+entityType, noFieldsToInferBody(entityType))
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "parsing form: "+err.Error(), http.StatusBadRequest)
		return
	}

	refTargets := refTargetsForSubmission(r.Context(), c, schema)
	values, errs := formengine.ParseFormValues(fields, r.PostForm, refTargets)
	basePath := entitiesBasePath(name, entityType)

	if len(errs) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		opts := resolveFormOptionsMaybeSchema(r.Context(), c, name, entityType, schema, values, errs)
		body := entityFormBody("New "+entityType, basePath, fields, opts, "")
		WriteModalAware(w, r, "New "+entityType, body)
		return
	}

	if _, err := c.Create(r.Context(), entityType, values); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		opts := resolveFormOptionsMaybeSchema(r.Context(), c, name, entityType, schema, values, nil)
		body := entityFormBody("New "+entityType, basePath, fields, opts, err.Error())
		WriteModalAware(w, r, "New "+entityType, body)
		return
	}
	rememberNewRefTargets(r.Context(), c, entityType, schema, r.PostForm, refTargets)

	http.Redirect(w, r, basePath, http.StatusSeeOther)
}

// ─── Edit ────────────────────────────────────────────────────────────────

func (h *EntitiesHandler) EditForm(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entityType := r.PathValue("type")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

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
	if err != nil && !isNotFoundError(err) {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}
	entity, err := c.Get(r.Context(), entityType, id)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	var fields []xclient.FieldDef
	if schema != nil {
		fields = schema.Fields
	} else {
		fields = inferFieldsFromEntities([]xclient.Entity{*entity})
	}

	basePath := entitiesBasePath(name, entityType)
	idStr := strconv.FormatInt(id, 10)
	opts := resolveFormOptionsMaybeSchema(r.Context(), c, name, entityType, schema, formengine.Values(entity.Data), nil)
	body := entityFormBody("Edit "+entityType+" #"+idStr, basePath+"/"+idStr, fields, opts, "")
	WriteModalAware(w, r, "Edit "+entityType, body)
}

func (h *EntitiesHandler) Update(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entityType := r.PathValue("type")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	schema, schemaErr := c.GetEntitySchema(r.Context(), entityType)
	if schemaErr != nil && !isNotFoundError(schemaErr) {
		writeUpstreamError(w, name, r.URL.Path, schemaErr)
		return
	}

	var fields []xclient.FieldDef
	if schema != nil {
		fields = schema.Fields
	} else {
		// No schema — infer from the specific row being edited, not an
		// arbitrary one, since its exact field set is what the
		// submission should be parsed against. A fetch failure here
		// (e.g. the row was deleted between EditForm and this
		// submission) is a real error, not "nothing to work with."
		current, err := c.Get(r.Context(), entityType, id)
		if err != nil {
			writeUpstreamError(w, name, r.URL.Path, err)
			return
		}
		fields = inferFieldsFromEntities([]xclient.Entity{*current})
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "parsing form: "+err.Error(), http.StatusBadRequest)
		return
	}

	refTargets := refTargetsForSubmission(r.Context(), c, schema)
	values, errs := formengine.ParseFormValues(fields, r.PostForm, refTargets)
	basePath := entitiesBasePath(name, entityType)
	idStr := strconv.FormatInt(id, 10)

	if len(errs) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		opts := resolveFormOptionsMaybeSchema(r.Context(), c, name, entityType, schema, values, errs)
		body := entityFormBody("Edit "+entityType+" #"+idStr, basePath+"/"+idStr, fields, opts, "")
		WriteModalAware(w, r, "Edit "+entityType, body)
		return
	}

	if _, err := c.Update(r.Context(), entityType, id, values); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		opts := resolveFormOptionsMaybeSchema(r.Context(), c, name, entityType, schema, values, nil)
		body := entityFormBody("Edit "+entityType+" #"+idStr, basePath+"/"+idStr, fields, opts, err.Error())
		WriteModalAware(w, r, "Edit "+entityType, body)
		return
	}
	rememberNewRefTargets(r.Context(), c, entityType, schema, r.PostForm, refTargets)

	http.Redirect(w, r, basePath, http.StatusSeeOther)
}

// entityFormBody renders the shared create/edit form shell: an optional
// general error banner (upstream validation failures that don't map to
// a specific field), formengine's rendered fields, and a submit button.
func entityFormBody(title, action string, fields []xclient.FieldDef, opts formengine.RenderOptions, generalErr string) mi.H {
	return func(b *mi.Builder) mi.Node {
		var errNode interface{}
		if generalErr != "" {
			errNode = b.Div(mi.Class("text-red-600 dark:text-red-400 text-sm mb-3"), generalErr)
		}
		return b.Div(
			b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white mb-4"), title),
			errNode,
			b.Form(mi.Attr("method", "post"), mi.Attr("action", action),
				formengine.RenderFields(fields, opts)(b),
				b.Div(mi.Style("margin-top:1.5rem;"),
					b.Button(mi.Type("submit"), mi.Class(btnPrimary), "Save"),
				),
			),
		)
	}
}

// ─── Delete ──────────────────────────────────────────────────────────────

func (h *EntitiesHandler) DeleteConfirm(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entityType := r.PathValue("type")
	id := r.PathValue("id")
	deleteURL := entitiesBasePath(name, entityType) + "/" + id + "/delete"
	WriteModalAware(w, r, "Delete "+entityType, DeleteConfirmBody(entityType+" #"+id, deleteURL))
}

func (h *EntitiesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entityType := r.PathValue("type")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	basePath := entitiesBasePath(name, entityType)
	if err := c.Delete(r.Context(), entityType, id); err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	http.Redirect(w, r, basePath, http.StatusSeeOther)
}
