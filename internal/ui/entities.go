// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	mi "github.com/ha1tch/minty"
	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/connstore"
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
func (h *EntitiesHandler) List(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	types, err := c.ListEntityTypes(r.Context())
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	body := func(b *mi.Builder) mi.Node {
		header := b.Div(mi.Class("mb-4"),
			b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white"), "Entities on "+name),
		)
		if len(types) == 0 {
			return b.Div(header, b.Div(mi.Class("text-center py-12 text-gray-500 dark:text-gray-400"), "No entity types registered on this instance."))
		}
		rows := make([]mi.Node, len(types))
		for i, t := range types {
			rows[i] = b.Tr(
				b.Td(mi.Class("px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm"),
					b.A(mi.Href("/connections/"+name+"/entities/"+t.Name), mi.Class("text-indigo-600 dark:text-indigo-400 hover:underline"), t.Name),
				),
			)
		}
		table := Table([]string{"Entity type"}, rows, "")(b)
		return b.Div(header, table)
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

	schema, err := c.GetEntitySchema(r.Context(), entityType)
	if err != nil {
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

	basePath := "/connections/" + name + "/entities/" + entityType
	cfg := ListPageConfig{Title: entityType, CreateLabel: "New " + entityType, CreateURL: basePath + "/new"}
	body := func(b *mi.Builder) mi.Node {
		importLink := b.A(mi.Href(basePath+"/import"), mi.Class(btnSecondary+" mb-3 inline-flex"), "Import from file")
		gridLink := b.A(mi.Href(basePath+"/grid"), mi.Class(btnSecondary+" mb-3 inline-flex"), "Grid view")
		links := b.Div(mi.Class("flex gap-2"), importLink, gridLink)
		return b.Div(links, ListPage(cfg, entityListTable(name, entityType, previewFields(schema.Fields), result))(b))
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
		if f.Type == "object" || f.Type == "array" {
			continue
		}
		out = append(out, f)
		if len(out) == maxPreviewColumns {
			break
		}
	}
	return out
}

func entityListTable(connName, entityType string, preview []xclient.FieldDef, result *xclient.ListResult) mi.H {
	return func(b *mi.Builder) mi.Node {
		if len(result.Entities) == 0 {
			return b.Div(mi.Class("text-center py-12 text-gray-500 dark:text-gray-400"), "No "+entityType+" entities yet.")
		}

		basePath := "/connections/" + connName + "/entities/" + entityType
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
				cells = append(cells, b.Td(mi.Class(td), previewValue(e.Data[f.Name])))
			}
			cells = append(cells,
				b.Td(mi.Class(td), b.A(mi.Href(editURL), mi.Class("text-indigo-600 dark:text-indigo-400 hover:underline"), "Edit")),
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
// JSON data (string, float64, bool, or nil/absent — object/array fields
// never reach here, previewFields already excludes them).
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

	schema, err := c.GetEntitySchema(r.Context(), entityType)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	basePath := "/connections/" + name + "/entities/" + entityType
	opts := resolveFormOptions(r.Context(), c, name, schema, nil, nil)
	body := entityFormBody("New "+entityType, basePath, schema.Fields, opts, "")
	WriteHTML(w, Page("New "+entityType, r.URL.Path, body))
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

	schema, err := c.GetEntitySchema(r.Context(), entityType)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "parsing form: "+err.Error(), http.StatusBadRequest)
		return
	}

	values, errs := formengine.ParseFormValues(schema.Fields, r.PostForm)
	basePath := "/connections/" + name + "/entities/" + entityType

	if len(errs) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		opts := resolveFormOptions(r.Context(), c, name, schema, values, errs)
		body := entityFormBody("New "+entityType, basePath, schema.Fields, opts, "")
		WriteHTML(w, Page("New "+entityType, r.URL.Path, body))
		return
	}

	if _, err := c.Create(r.Context(), entityType, values); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		opts := resolveFormOptions(r.Context(), c, name, schema, values, nil)
		body := entityFormBody("New "+entityType, basePath, schema.Fields, opts, err.Error())
		WriteHTML(w, Page("New "+entityType, r.URL.Path, body))
		return
	}

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
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}
	entity, err := c.Get(r.Context(), entityType, id)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	basePath := "/connections/" + name + "/entities/" + entityType
	idStr := strconv.FormatInt(id, 10)
	opts := resolveFormOptions(r.Context(), c, name, schema, formengine.Values(entity.Data), nil)
	body := entityFormBody("Edit "+entityType+" #"+idStr, basePath+"/"+idStr, schema.Fields, opts, "")
	WriteHTML(w, Page("Edit "+entityType, r.URL.Path, body))
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

	schema, err := c.GetEntitySchema(r.Context(), entityType)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "parsing form: "+err.Error(), http.StatusBadRequest)
		return
	}

	values, errs := formengine.ParseFormValues(schema.Fields, r.PostForm)
	basePath := "/connections/" + name + "/entities/" + entityType
	idStr := strconv.FormatInt(id, 10)

	if len(errs) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		opts := resolveFormOptions(r.Context(), c, name, schema, values, errs)
		body := entityFormBody("Edit "+entityType+" #"+idStr, basePath+"/"+idStr, schema.Fields, opts, "")
		WriteHTML(w, Page("Edit "+entityType, r.URL.Path, body))
		return
	}

	if _, err := c.Update(r.Context(), entityType, id, values); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		opts := resolveFormOptions(r.Context(), c, name, schema, values, nil)
		body := entityFormBody("Edit "+entityType+" #"+idStr, basePath+"/"+idStr, schema.Fields, opts, err.Error())
		WriteHTML(w, Page("Edit "+entityType, r.URL.Path, body))
		return
	}

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
	deleteURL := "/connections/" + name + "/entities/" + entityType + "/" + id + "/delete"
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

	basePath := "/connections/" + name + "/entities/" + entityType
	if err := c.Delete(r.Context(), entityType, id); err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	http.Redirect(w, r, basePath, http.StatusSeeOther)
}
