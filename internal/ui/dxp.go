// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// dxp.go implements T-25's design (docs/proposals/dxp-transactions.md):
// a def picker plus a dynamically-generated binding-fill form,
// deliberately its own view rather than a fourth query-editor mode —
// a DXP transaction has no query language, only a chosen definition
// and the parameter values its participants reference.
package ui

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"

	mi "github.com/ha1tch/minty"
	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/connstore"
	"github.com/ha1tch/xoluman/internal/modules"
	"github.com/ha1tch/xoluman/internal/xoluext"
)

// RegisterDXPModule registers DXP's routes. No nav Label — reached
// per-connection via the sidebar, same reasoning as Entities/Blobs/
// Query.
func RegisterDXPModule(reg *modules.Registry, store connstore.Store) {
	h := &dxpHandler{store: store}
	reg.Register(modules.Module{
		ID: "dxp",
		MountRoutes: func(mux *http.ServeMux) {
			mux.HandleFunc("GET /connections/{name}/dxp", h.View)
			mux.HandleFunc("GET /connections/{name}/dxp/defs", h.ListDefs)
			mux.HandleFunc("GET /connections/{name}/dxp/defs/{id}", h.GetDef)
			mux.HandleFunc("POST /connections/{name}/dxp/run", h.Run)
		},
	})
}

type dxpHandler struct {
	store connstore.Store
}

func (h *dxpHandler) clientFor(r *http.Request, name string) (*xclient.Client, error) {
	conn, err := h.store.Get(r.Context(), name)
	if err != nil {
		return nil, err
	}
	return xoluext.BuildClient(conn), nil
}

// View renders the DXP page shell — a Lit component (dxp-editor.js)
// does everything else via the JSON endpoints below, same
// architecture as query-editor.js.
func (h *dxpHandler) View(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := h.store.Get(r.Context(), name); err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	basePath := "/connections/" + url.PathEscape(name) + "/dxp"
	body := func(b *mi.Builder) mi.Node {
		return b.Div(
			b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white mb-4"), "DXP transactions — "+name),
			mi.Raw(`<xolu-dxp-editor defs-url="`+htmlEscape(basePath+"/defs")+`" run-url="`+htmlEscape(basePath+"/run")+`"></xolu-dxp-editor>`),
		)
	}
	WriteHTML(w, dxpPage("DXP — "+name, r.URL.Path, body))
}

func dxpPage(title, activePath string, body mi.H) mi.H {
	return func(b *mi.Builder) mi.Node {
		extraHead := []mi.Node{
			b.Script(mi.Attr("type", "importmap"), mi.Raw(`{"imports":{"lit":"/static/vendor/lit@3.js"}}`)),
			b.Script(mi.Attr("type", "module"), mi.Attr("src", "/static/js/dxp-editor.js")),
		}
		return PageWithHead(title, activePath, extraHead, body)(b)
	}
}

// dxpDefSummary is what ListDefs returns per definition — just enough
// for a picker to populate itself, matching DxpDefList's own
// deliberately narrow summary shape.
type dxpDefSummary struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

func (h *dxpHandler) ListDefs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c, err := h.clientFor(r, name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	result, err := c.DxpDefList(r.Context())
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	summaries := make([]dxpDefSummary, len(result.Definitions))
	for i, d := range result.Definitions {
		summaries[i] = dxpDefSummary{ID: d.ID, Name: d.Name, CreatedAt: d.CreatedAt}
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Name < summaries[j].Name })

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(summaries)
}

// dxpDefDetail is what GetDef returns — the def's own participants
// (so the UI can show what it actually does, not just its name) plus
// Bindings, the extracted, de-duplicated, sorted list of every
// binding name any participant's Params reference via {"$ref": "..."}.
// This walk lives here, server-side, rather than being reimplemented
// in JS — one implementation of "what does this def need," not two
// that could drift apart.
type dxpDefDetail struct {
	ID             int64                    `json:"id"`
	Name           string                   `json:"name"`
	Pattern        string                   `json:"pattern"`
	Participants   []xclient.DxpParticipant `json:"participants"`
	Bindings       []string                 `json:"bindings"`
	BindingsSchema map[string]interface{}   `json:"bindingsSchema,omitempty"`
}

func (h *dxpHandler) GetDef(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	c, err := h.clientFor(r, name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	def, err := c.DxpDefGet(r.Context(), id)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	var participants []xclient.DxpParticipant
	if def.Spec != nil {
		participants = def.Spec.Participants
	}

	detail := dxpDefDetail{
		ID:             def.ID,
		Name:           def.Name,
		Participants:   participants,
		Bindings:       extractBindingNames(participants),
		BindingsSchema: def.BindingsSchema,
	}
	if def.Spec != nil {
		detail.Pattern = def.Spec.Pattern
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(detail)
}

// extractBindingNames walks every participant's Params, collecting
// every distinct name referenced via {"$ref": "<name>"} — including
// nested inside an array or another object, not just a top-level
// param value, since Params is map[string]interface{} and a def's
// author is free to nest a $ref anywhere valid JSON allows one.
// Returns a sorted, de-duplicated list — sorted so the generated form
// has a stable field order across repeated GetDef calls, not
// whatever order a map happened to iterate in.
func extractBindingNames(participants []xclient.DxpParticipant) []string {
	seen := make(map[string]bool)
	for _, p := range participants {
		for _, v := range p.Params {
			walkForRefs(v, seen)
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func walkForRefs(v interface{}, seen map[string]bool) {
	switch t := v.(type) {
	case map[string]interface{}:
		if len(t) == 1 {
			if ref, ok := t["$ref"].(string); ok && ref != "" {
				seen[ref] = true
				return
			}
		}
		for _, sub := range t {
			walkForRefs(sub, seen)
		}
	case []interface{}:
		for _, sub := range t {
			walkForRefs(sub, seen)
		}
	}
}

// dxpRunRequest is what the Lit component POSTs to Run.
type dxpRunRequest struct {
	DefID    int64                  `json:"defId"`
	Bindings map[string]interface{} `json:"bindings"`
}

// Run instantiates and dispatches a transaction. A non-committed
// outcome (released/expired) is a normal DxpTxn response, not a Go
// error — confirmed directly against DxpTxnCreate's own doc comment —
// so this always returns 200 with the real Status/Reason for the
// caller to render, only using writeUpstreamError for a genuine
// transport/validation failure (a bad DefID, bindings failing the
// def's own schema before a transaction was even attempted).
func (h *dxpHandler) Run(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c, err := h.clientFor(r, name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	var req dxpRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.DefID <= 0 {
		http.Error(w, "defId must be a positive integer", http.StatusBadRequest)
		return
	}

	txn, err := c.DxpTxnCreate(r.Context(), xclient.DxpTxnCreateRequest{DefID: req.DefID, Bindings: req.Bindings})
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(txn)
}
