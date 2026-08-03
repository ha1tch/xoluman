// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	mi "github.com/ha1tch/minty"

	"github.com/ha1tch/xoluman/internal/connstore"
	"github.com/ha1tch/xoluman/internal/modules"
	"github.com/ha1tch/xoluman/internal/xoluext"
)

func init() {
	// Populates the shared nav-only registry (layout.go's package-level
	// registry, used only by Page() for nav rendering — its MountRoutes
	// closures are never invoked, so the nil store below is harmless).
	// server.New builds its own fresh registry per call for actual route
	// mounting, via RegisterConnectionsModule with the real store —
	// necessary because http.ServeMux panics on a duplicate pattern
	// registration, and the shared registry would otherwise accumulate
	// one "connections" entry per server.New call across a test binary.
	RegisterConnectionsModule(registry, nil)
}

// RegisterConnectionsModule registers the Connections module — nav
// entry and routes together — onto reg, backed by store.
func RegisterConnectionsModule(reg *modules.Registry, store connstore.Store) {
	conns := NewConnectionsHandler(store)
	reg.Register(modules.Module{
		ID: "connections", Label: "Connections", URL: "/connections",
		ActivePrefix: "/connections", Order: 0,
		MountRoutes: func(mux *http.ServeMux) {
			mux.HandleFunc("GET /connections", conns.List)
			mux.HandleFunc("GET /connections/new", conns.NewForm)
			mux.HandleFunc("POST /connections", conns.Create)
			mux.HandleFunc("GET /connections/{name}/delete-confirm", conns.DeleteConfirm)
			mux.HandleFunc("POST /connections/{name}/delete", conns.Delete)
			mux.HandleFunc("POST /connections/{name}/test", conns.Test)
		},
	})
}

// ConnectionsHandler serves every /connections route. store is the only
// dependency — no server-wide state beyond it is needed for this page.
type ConnectionsHandler struct {
	store connstore.Store
}

// NewConnectionsHandler returns a ConnectionsHandler backed by store.
func NewConnectionsHandler(store connstore.Store) *ConnectionsHandler {
	return &ConnectionsHandler{store: store}
}

// authModeOptions is the fixed set offered in the add-connection form,
// in the same order as connstore's AuthMode constants.
var authModeOptions = []struct {
	Value connstore.AuthMode
	Label string
}{
	{connstore.AuthNone, "None"},
	{connstore.AuthAPIKey, "API key"},
	{connstore.AuthBearer, "Bearer token"},
	{connstore.AuthJWT, "JWT"},
}

// formInputClass and formLabelClass match Seam's own form field styling.
const (
	formLabelClass = "block mt-3 mb-1 text-sm text-gray-600 dark:text-gray-400"
	formInputClass = "w-full max-w-md px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 text-sm"
)

// List renders the connections listing page: header with a modal-opening
// "+ New connection" button (ListPage), and the connections table.
func (h *ConnectionsHandler) List(w http.ResponseWriter, r *http.Request) {
	conns, err := h.store.List(r.Context())
	if err != nil {
		http.Error(w, "listing connections: "+err.Error(), http.StatusInternalServerError)
		return
	}

	cfg := ListPageConfig{Title: "Connections", CreateLabel: "New connection", CreateURL: "/connections/new"}
	body := ListPage(cfg, connectionsTable(conns))
	WriteHTML(w, Page("Connections", r.URL.Path, body))
}

func connectionsTable(conns []connstore.Connection) mi.H {
	return func(b *mi.Builder) mi.Node {
		rows := make([]mi.Node, len(conns))
		for i, c := range conns {
			rows[i] = connectionRow(b, c)
		}
		columns := []string{"Name", "Base URL", "Auth", "Tenant", "Status", ""}
		return Table(columns, rows, "No connections yet. Add one to get started.")(b)
	}
}

func connectionRow(b *mi.Builder, c connstore.Connection) mi.Node {
	tenant := c.Tenant
	if tenant == "" {
		tenant = "—"
	}
	td := "px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm text-gray-700 dark:text-gray-300"
	deleteConfirmURL := "/connections/" + c.Name + "/delete-confirm"
	return b.Tr(mi.ID("conn-row-"+c.Name),
		b.Td(mi.Class(td), c.Name),
		b.Td(mi.Class(td), c.BaseURL),
		b.Td(mi.Class(td), string(c.AuthMode)),
		b.Td(mi.Class(td), tenant),
		b.Td(mi.Class(td), mi.ID("status-"+c.Name), "untested"),
		b.Td(mi.Class(td),
			b.Div(mi.Class("flex gap-2"),
				b.A(mi.Href("/connections/"+c.Name+"/entities"), mi.Class(btnSecondary), "Entities"),
				b.Button(
					mi.Type("button"), mi.Class(btnSecondary),
					mi.HxPost("/connections/"+c.Name+"/test"),
					mi.HxTarget("#status-"+c.Name),
					mi.HxSwap("innerHTML"),
					"Test",
				),
				ModalTriggerButton("Delete", "Delete connection", deleteConfirmURL, btnDanger)(b),
			),
		),
	)
}

// NewForm serves the add-connection form: bare fragment when loaded into
// the modal (the normal path, via the "+ New connection" button), full
// page shell on a direct navigation.
func (h *ConnectionsHandler) NewForm(w http.ResponseWriter, r *http.Request) {
	WriteModalAware(w, r, "New connection", connectionFormBody("", nil))
}

// connectionFormBody renders the add-connection form. formErr, when
// non-empty, is shown above the fields — used when Create redisplays
// the form after a validation failure rather than losing the person's
// input.
func connectionFormBody(formErr string, values url.Values) mi.H {
	return func(b *mi.Builder) mi.Node {
		get := func(key string) string {
			return values.Get(key)
		}

		var errNode interface{} = nil
		if formErr != "" {
			errNode = b.Div(mi.Class("text-red-600 dark:text-red-400 text-sm mb-3"), formErr)
		}

		authOpts := make([]interface{}, 0, len(authModeOptions))
		for _, o := range authModeOptions {
			attrs := []interface{}{mi.Value(string(o.Value)), o.Label}
			if get("auth_mode") == string(o.Value) {
				attrs = append(attrs, mi.Selected())
			}
			authOpts = append(authOpts, b.Option(attrs...))
		}

		return b.Div(
			errNode,
			b.Form(mi.Attr("method", "post"), mi.Attr("action", "/connections"),
				b.Label(mi.For("name"), mi.Class(formLabelClass), "Name"),
				b.Input(mi.Type("text"), mi.ID("name"), mi.Name("name"), mi.Value(get("name")), mi.Required(), mi.Class(formInputClass)),

				b.Label(mi.For("base_url"), mi.Class(formLabelClass), "Base URL"),
				b.Input(mi.Type("text"), mi.ID("base_url"), mi.Name("base_url"), mi.Value(get("base_url")),
					mi.Placeholder("http://localhost:8080"), mi.Required(), mi.Class(formInputClass)),

				b.Label(mi.For("auth_mode"), mi.Class(formLabelClass), "Auth mode"),
				b.Select(append([]interface{}{mi.ID("auth_mode"), mi.Name("auth_mode"), mi.Class(formInputClass)}, authOpts...)...),

				b.Label(mi.For("token"), mi.Class(formLabelClass), "Token"),
				b.Input(mi.Type("password"), mi.ID("token"), mi.Name("token"), mi.Value(get("token")),
					mi.Placeholder("Leave blank for auth mode \"None\""), mi.Class(formInputClass)),

				b.Label(mi.For("tenant"), mi.Class(formLabelClass), "Tenant (optional)"),
				b.Input(mi.Type("text"), mi.ID("tenant"), mi.Name("tenant"), mi.Value(get("tenant")), mi.Class(formInputClass)),

				b.Div(mi.Class("mt-6"),
					b.Button(mi.Type("submit"), mi.Class(btnPrimary), "Save connection"),
				),
			),
		)
	}
}

// Create validates and saves a new (or updated, since Save upserts)
// connection from a submitted form. The form is a plain POST (not htmx)
// so both success (redirect) and failure (redisplay) are real
// navigations — WriteModalAware's full-page branch is always what fires
// here, which is correct: the browser is replacing the whole document
// either way, modal or not.
func (h *ConnectionsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parsing form: "+err.Error(), http.StatusBadRequest)
		return
	}

	conn := connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{
			Name:     r.PostForm.Get("name"),
			BaseURL:  r.PostForm.Get("base_url"),
			AuthMode: connstore.AuthMode(r.PostForm.Get("auth_mode")),
			Tenant:   r.PostForm.Get("tenant"),
		},
		Token: r.PostForm.Get("token"),
	}

	if _, err := h.store.Save(r.Context(), conn); err != nil {
		msg := friendlySaveError(err)
		w.WriteHeader(http.StatusUnprocessableEntity)
		WriteModalAware(w, r, "New connection", connectionFormBody(msg, r.PostForm))
		return
	}

	http.Redirect(w, r, "/connections", http.StatusSeeOther)
}

func friendlySaveError(err error) string {
	switch {
	case errors.Is(err, connstore.ErrInvalidName):
		return "Name is required."
	case errors.Is(err, connstore.ErrInvalidAuthMode):
		return "Choose a valid auth mode."
	default:
		return "Could not save connection: " + err.Error()
	}
}

// DeleteConfirm serves the delete-confirmation modal body for the named
// connection.
func (h *ConnectionsHandler) DeleteConfirm(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	WriteModalAware(w, r, "Delete connection", DeleteConfirmBody(name, "/connections/"+name+"/delete"))
}

// Delete removes the named connection and returns to the list. Deleting
// an already-gone connection (ErrNotFound) is treated as success — the
// end state the person wanted is achieved either way.
func (h *ConnectionsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.store.Delete(r.Context(), name); err != nil && !errors.Is(err, connstore.ErrNotFound) {
		http.Error(w, "deleting connection: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/connections", http.StatusSeeOther)
}

// Test is an htmx fragment endpoint: builds a client for the named
// connection and calls Health, returning a small status span. Swapped
// into the row's status cell by the Test button's hx-target. Unlike the
// create/delete flows this is never a full-page navigation — it's always
// a small in-place htmx swap, so it uses WriteFragment directly rather
// than WriteModalAware.
func (h *ConnectionsHandler) Test(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	conn, err := h.store.Get(r.Context(), name)
	if err != nil {
		WriteFragment(w, statusFragment(false, "not found"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	c := xoluext.BuildClient(conn)
	if err := c.Health(ctx); err != nil {
		WriteFragment(w, statusFragment(false, err.Error()))
		return
	}
	WriteFragment(w, statusFragment(true, "ok"))
}

func statusFragment(ok bool, detail string) mi.H {
	return func(b *mi.Builder) mi.Node {
		class := "text-green-600 dark:text-green-400"
		label := "reachable"
		if !ok {
			class = "text-red-600 dark:text-red-400"
			label = "unreachable"
		}
		return b.Span(mi.Class(class), mi.Title(detail), label)
	}
}

// WriteFragment renders template and writes it as a bare htmx-swappable
// fragment — no page shell, unlike WriteHTML.
func WriteFragment(w http.ResponseWriter, template mi.H) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = mi.Render(template, w)
}
