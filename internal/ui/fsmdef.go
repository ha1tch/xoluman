// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// fsmdef.go implements T-14 (the FSM def module) plus the two specific
// asks that came with it: validate a machine before submission using
// github.com/ha1tch/fsm-toolkit, and persist a visual diagram the same
// way fsm-toolkit's own fsmedit tool does.
//
// On the validation ask: fsm-toolkit's *fsm.FSM is a genuinely
// different, richer model than xolu's own client.MachineSpec — built
// for circuit/hardware simulation (Classes, Nets, a dfa/nfa/moore/mealy
// Type system), not xolu's variable/guard/GC-policy model. There's no
// lossless round-trip between the two. What this file does instead:
// convert the structural parts that DO map cleanly (states, the
// initial state, transitions' from/to/input/output) into a *fsm.FSM,
// then run fsm-toolkit's own Validate()/Analyse() against that —
// catching typos in state names, an unreachable initial state, a
// transition pointing at a state that doesn't exist, dead states with
// no outgoing transitions, before ever making a network call to xolu.
// Anything xolu-specific fsm-toolkit has no concept of at all (guards,
// variable Set clauses, GC policy) is called out explicitly as a
// skipped-check note, not silently ignored — the person editing should
// know this is a structural check, not a full semantic one, and that
// xolu's own ValidateMachineDef (a real network call, but the
// authoritative check) still runs before anything is actually saved.
//
// On the layout ask: fsm-toolkit's fsmedit persists visual state
// positions as a *separate* structure from the abstract machine
// (pkg/fsmfile's Layout type — a plain state-name -> {x,y} map plus a
// canvas offset, versioned) rather than folding position data into the
// FSM itself. This file follows the same separation, not the same
// file format (fsmedit's .fsm files are zip archives of a hex-encoded
// machine plus layout.toml — built for local files on a TUI tool, not
// a fit for a web app talking to a remote xolu instance): a machine
// definition's layout is stored as its own xoluman-owned bookkeeping
// entity (xoluman_fsm_layout), the same established pattern as
// xoluman_field_meta and xoluman_saved_query, keyed by the machine
// def's own ID and holding the identical conceptual shape fsmedit
// itself uses (per-state x/y, a canvas offset, a version number).
package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	fsmtk "github.com/ha1tch/fsm-toolkit/pkg/fsm"
	mi "github.com/ha1tch/minty"
	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/connstore"
	"github.com/ha1tch/xoluman/internal/modules"
	"github.com/ha1tch/xoluman/internal/xoluext"
)

// fsmToolkitFSM converts a xolu MachineSpec into fsm-toolkit's own
// *fsm.FSM for structural validation. Always builds a Mealy machine
// regardless of spec.Determinism — fsm-toolkit's Type is a different
// axis (dfa/nfa/moore/mealy, about epsilon-transitions and output
// placement) from xolu's own Determinism (strict/loose/firstmatch,
// about ambiguity resolution); Mealy is the closest fit since it's
// the only fsm-toolkit type whose transitions carry their own output,
// matching TransitionDef.Output directly, and its Validate()/Analyse()
// checks are a superset of what a plain DFA's would catch.
//
// notes lists every structural feature xolu's spec used that
// fsm-toolkit's model has no equivalent for (guards, variable Set
// clauses) — surfaced to the caller so "this passed validation" is
// never mistaken for "this was fully checked."
func fsmToolkitFSM(spec xclient.MachineSpec) (f *fsmtk.FSM, notes []string, err error) {
	f = fsmtk.New(fsmtk.TypeMealy)
	f.Name = spec.Name
	f.Description = spec.Description
	f.Initial = spec.Initial

	for name := range spec.States {
		f.AddState(name)
	}
	for _, o := range spec.OutputAlphabet {
		f.AddOutput(o)
	}

	for i, t := range spec.Transitions {
		froms, ferr := t.FromStates()
		if ferr != nil {
			return nil, notes, fmt.Errorf("transition %d: %w", i, ferr)
		}
		if t.Guard != "" {
			notes = append(notes, fmt.Sprintf("transition %d (input %q): guard %q not checked — xolu-specific, fsm-toolkit has no guard concept", i, t.Input, t.Guard))
		}
		if len(t.Set) > 0 {
			notes = append(notes, fmt.Sprintf("transition %d (input %q): variable assignment not checked — xolu-specific, fsm-toolkit has no variable concept", i, t.Input))
		}

		input := t.Input
		if input != "" {
			f.AddInput(input)
		}
		var output *string
		if t.Output != "" {
			out := t.Output
			output = &out
		}
		for _, from := range froms {
			f.AddTransition(from, &input, []string{t.To}, output)
		}
	}
	if len(spec.Variables) > 0 {
		notes = append(notes, fmt.Sprintf("%d machine variable(s) not checked — xolu-specific, fsm-toolkit has no variable concept", len(spec.Variables)))
	}
	if len(spec.InputQueries) > 0 {
		notes = append(notes, fmt.Sprintf("%d input quer(ies) not checked — xolu-specific OQL-input binding, fsm-toolkit has no equivalent", len(spec.InputQueries)))
	}

	return f, notes, nil
}

// localValidation is the JSON shape ValidateLocal returns — mirrors
// xolu's own MachineDefValidation in spirit (Valid/Errors) with an
// added Warnings list (fsm-toolkit's Analyse() output: unreachable
// states, dead states — real issues that don't make a spec invalid,
// same as xolu's own post-creation MachineDefAnalysis.Warnings) and
// SkippedChecks (see fsmToolkitFSM's own doc comment).
type localValidation struct {
	Valid         bool     `json:"valid"`
	Errors        []string `json:"errors,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
	SkippedChecks []string `json:"skippedChecks,omitempty"`
}

// validateSpecLocally runs fsm-toolkit's Validate()/Analyse() against
// spec without any network call — the actual "before submission" part
// of the ask. A spec that fails the conversion itself (a malformed
// From field) is reported as a validation error, not a 500 — from the
// caller's perspective a spec that can't even be structurally
// understood is exactly as invalid as one that fails Validate().
func validateSpecLocally(spec xclient.MachineSpec) localValidation {
	f, notes, err := fsmToolkitFSM(spec)
	if err != nil {
		return localValidation{Valid: false, Errors: []string{err.Error()}}
	}
	result := localValidation{SkippedChecks: notes}
	if verr := f.Validate(); verr != nil {
		result.Valid = false
		result.Errors = []string{verr.Error()}
		return result
	}
	result.Valid = true
	for _, w := range f.Analyse() {
		result.Warnings = append(result.Warnings, w.Message)
	}
	return result
}

// ─── HTTP layer ─────────────────────────────────────────────────────────

// fsmLayoutEntityType is the xoluman-owned bookkeeping entity type
// holding visual layouts — same established mechanism as
// xoluman_field_meta/xoluman_saved_query, not a new xolu API. Keyed by
// the machine def's own numeric ID (definition_id), one row per
// machine.
const fsmLayoutEntityType = "xoluman_fsm_layout"

// stateLayout mirrors fsm-toolkit's own pkg/fsmfile.StateLayout shape
// (x, y) — deliberately the same field names and meaning, not a
// xoluman-specific reinvention of what a state's visual position is.
type stateLayout struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// fsmLayout mirrors fsm-toolkit's own pkg/fsmfile.Layout shape
// (version, canvas offset, per-state positions) — the same
// conceptual structure fsmedit persists, stored as xoluman's own
// bookkeeping data rather than fsmedit's zip+hex+TOML file format,
// which is built for local files on a TUI tool, not a web app talking
// to a remote xolu instance.
type fsmLayout struct {
	Version       int                    `json:"version"`
	CanvasOffsetX int                    `json:"canvasOffsetX"`
	CanvasOffsetY int                    `json:"canvasOffsetY"`
	States        map[string]stateLayout `json:"states"`
}

func RegisterFSMModule(reg *modules.Registry, store connstore.Store) {
	h := &fsmHandler{store: store}
	reg.Register(modules.Module{
		ID: "fsm",
		MountRoutes: func(mux *http.ServeMux) {
			mux.HandleFunc("GET /connections/{name}/fsm", h.List)
			mux.HandleFunc("GET /connections/{name}/fsm/new", h.NewForm)
			mux.HandleFunc("GET /connections/{name}/fsm/{id}", h.View)
			mux.HandleFunc("GET /connections/{name}/fsm/{id}/data", h.GetData)
			mux.HandleFunc("POST /connections/{name}/fsm", h.Create)
			mux.HandleFunc("PUT /connections/{name}/fsm/{id}", h.Update)
			mux.HandleFunc("DELETE /connections/{name}/fsm/{id}", h.Delete)
			mux.HandleFunc("POST /connections/{name}/fsm/validate", h.ValidateLocal)
			mux.HandleFunc("PUT /connections/{name}/fsm/{id}/layout", h.SaveLayout)
		},
	})
}

type fsmHandler struct {
	store connstore.Store
}

func (h *fsmHandler) clientFor(r *http.Request, name string) (*xclient.Client, error) {
	conn, err := h.store.Get(r.Context(), name)
	if err != nil {
		return nil, err
	}
	return xoluext.BuildClient(conn), nil
}

// writeUpstreamErrorJSON is writeUpstreamError's counterpart for this
// file's JSON endpoints — Create/Update/Delete/GetData/ValidateLocal/
// SaveLayout are all called via fetch() from fsm-editor.js, which
// expects a JSON error body it can actually read, not a full HTML
// error page (writeUpstreamError's own, correct behavior for the
// page-rendering handlers elsewhere in this codebase — not changed
// here, since that's shared across every other handler and not this
// file's problem to fix). Preserves xolu's own HTTP status when the
// error carries one (*xclient.Error), falling back to 502 for a
// genuine transport failure, matching writeUpstreamError's own
// fallback.
func writeUpstreamErrorJSON(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	if xerr, ok := err.(*xclient.Error); ok && xerr.HTTPStatus != 0 {
		status = xerr.HTTPStatus
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func fsmPage(title, activePath string, body mi.H) mi.H {
	return func(b *mi.Builder) mi.Node {
		extraHead := []mi.Node{
			b.Script(mi.Attr("type", "importmap"), mi.Raw(`{"imports":{"lit":"/static/vendor/lit@3.js"}}`)),
			// Plain script, not a module — fsm-canvas-engine.js (Seam's
			// own extended fork of Wallace's Finite State Machine
			// Designer, reused directly rather than a from-scratch
			// canvas; see fsm-editor.js's own top comment) exposes its
			// API as globals (initFSM, draw, getBackupData, ...), which
			// the Lit shell below calls directly. A plain <script> in
			// <head> executes immediately, before the deferred module
			// script that needs those globals to already exist.
			b.Script(mi.Attr("src", "/static/js/fsm-canvas-engine.js")),
			b.Script(mi.Attr("type", "module"), mi.Attr("src", "/static/js/fsm-editor.js")),
		}
		return PageWithHead(title, activePath, extraHead, body)(b)
	}
}

// List renders every registered machine definition for this
// connection, newest-looking-first-by-name (ListMachineDefs' own
// order, not re-sorted — matches what DXP's own def picker does).
func (h *fsmHandler) List(w http.ResponseWriter, r *http.Request) {
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
	defs, err := c.ListMachineDefs(r.Context())
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	basePath := "/connections/" + url.PathEscape(name) + "/fsm"
	body := func(b *mi.Builder) mi.Node {
		td := "px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm text-gray-700 dark:text-gray-300"
		rows := make([]mi.Node, 0, len(defs))
		for _, d := range defs {
			rows = append(rows, b.Tr(
				b.Td(mi.Class(td), strconv.FormatInt(d.ID, 10)),
				b.Td(mi.Class(td), b.A(mi.Href(basePath+"/"+strconv.FormatInt(d.ID, 10)), mi.Class("text-indigo-600 dark:text-indigo-400 hover:underline"), d.Name)),
				b.Td(mi.Class(td), d.CreatedAt),
			))
		}
		return b.Div(
			b.Div(mi.Class("flex items-center justify-between mb-4"),
				b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white"), "State machine definitions — "+name),
				b.A(mi.Href(basePath+"/new"), mi.Class(btnPrimary), "+ New machine"),
			),
			Table([]string{"ID", "Name", "Created"}, rows, "No machine definitions yet.")(b),
		)
	}
	WriteHTML(w, fsmPage("FSM definitions — "+name, r.URL.Path, body))
}

// NewForm and View both render the same editor shell — the Lit
// component (fsm-editor.js) fetches its own data (empty for a new
// machine, the real def+layout for an existing one) via defs-url,
// distinguishing the two entirely client-side rather than needing two
// server-rendered variants.
func (h *fsmHandler) NewForm(w http.ResponseWriter, r *http.Request) {
	h.renderEditor(w, r, 0)
}

func (h *fsmHandler) View(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	h.renderEditor(w, r, id)
}

func (h *fsmHandler) renderEditor(w http.ResponseWriter, r *http.Request, id int64) {
	name := r.PathValue("name")
	if _, err := h.store.Get(r.Context(), name); err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	basePath := "/connections/" + url.PathEscape(name) + "/fsm"
	idStr := strconv.FormatInt(id, 10)
	title := "New machine — " + name
	dataURL := basePath + "/new/data" // unused by the component for id==0, but kept well-formed
	saveURL := basePath
	if id > 0 {
		title = "Edit machine #" + idStr + " — " + name
		dataURL = basePath + "/" + idStr + "/data"
		saveURL = basePath + "/" + idStr
	}

	body := func(b *mi.Builder) mi.Node {
		return mi.Raw(fmt.Sprintf(
			`<xolu-fsm-editor id-value="%d" data-url="%s" save-url="%s" validate-url="%s" layout-url="%s"></xolu-fsm-editor>`,
			id, htmlEscape(dataURL), htmlEscape(saveURL), htmlEscape(basePath+"/validate"), htmlEscape(basePath+"/"+idStr+"/layout"),
		))
	}
	WriteHTML(w, fsmPage(title, r.URL.Path, body))
}

// fsmDefData is what GetData returns — the raw MachineDef (nil Spec
// fields all present, even for a brand-new machine — the component
// treats id==0 as "start empty" itself rather than this endpoint
// needing two response shapes) plus whatever layout has been saved
// for it, if any.
type fsmDefData struct {
	Def    *xclient.MachineDef `json:"def,omitempty"`
	Layout *fsmLayout          `json:"layout,omitempty"`
}

func (h *fsmHandler) GetData(w http.ResponseWriter, r *http.Request) {
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
		writeUpstreamErrorJSON(w, err)
		return
	}

	def, err := c.GetMachineDef(r.Context(), id)
	if err != nil {
		writeUpstreamErrorJSON(w, err)
		return
	}
	layout := h.loadLayout(r.Context(), c, id)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fsmDefData{Def: def, Layout: layout})
}

func (h *fsmHandler) loadLayout(ctx context.Context, c *xclient.Client, defID int64) *fsmLayout {
	result, err := c.List(ctx, fsmLayoutEntityType, &xclient.ListParams{Limit: 1000})
	if err != nil {
		return nil // no layout saved yet (or entity type doesn't exist), not an error the caller needs to see
	}
	for _, e := range result.Entities {
		id, _ := e.Data["definition_id"].(float64)
		if int64(id) != defID {
			continue
		}
		raw, ok := e.Data["layout"].(map[string]any)
		if !ok {
			continue
		}
		b, err := json.Marshal(raw)
		if err != nil {
			continue
		}
		var lay fsmLayout
		if err := json.Unmarshal(b, &lay); err != nil {
			continue
		}
		return &lay
	}
	return nil
}

func (h *fsmHandler) Create(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c, err := h.clientFor(r, name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamErrorJSON(w, err)
		return
	}

	var spec xclient.MachineSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	result, err := c.CreateMachineDef(r.Context(), spec)
	if err != nil {
		writeUpstreamErrorJSON(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (h *fsmHandler) Update(w http.ResponseWriter, r *http.Request) {
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
		writeUpstreamErrorJSON(w, err)
		return
	}

	var spec xclient.MachineSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	result, err := c.ReplaceMachineDef(r.Context(), id, spec)
	if err != nil {
		writeUpstreamErrorJSON(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (h *fsmHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
		writeUpstreamErrorJSON(w, err)
		return
	}
	if err := c.DeleteMachineDef(r.Context(), id); err != nil {
		writeUpstreamErrorJSON(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ValidateLocal is the actual "before submission" endpoint — runs
// fsm-toolkit's own Validate()/Analyse() against the submitted spec
// with no call to xolu at all, so it's meaningfully faster than
// xolu's own ValidateMachineDef and works from inside the editor
// without needing the person to have saved anything yet.
func (h *fsmHandler) ValidateLocal(w http.ResponseWriter, r *http.Request) {
	var spec xclient.MachineSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := validateSpecLocally(spec)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// SaveLayout persists the visual diagram — see this file's own top
// doc comment for why this is xoluman's own bookkeeping entity rather
// than anything xolu's API knows about. One document per machine def
// ID: an existing layout is replaced by deleting it first rather than
// leaving stale rows behind (this entity type has no natural
// upsert-by-key operation to use instead).
func (h *fsmHandler) SaveLayout(w http.ResponseWriter, r *http.Request) {
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
		writeUpstreamErrorJSON(w, err)
		return
	}

	var layout fsmLayout
	if err := json.NewDecoder(r.Body).Decode(&layout); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if layout.Version == 0 {
		layout.Version = 1
	}

	if result, err := c.List(r.Context(), fsmLayoutEntityType, &xclient.ListParams{Limit: 1000}); err == nil {
		for _, e := range result.Entities {
			existingID, _ := e.Data["definition_id"].(float64)
			if int64(existingID) == id {
				_ = c.Delete(r.Context(), fsmLayoutEntityType, e.ID) // best-effort — a stale row left behind is a minor future confusion, not worth failing the save over
			}
		}
	}

	_, err = c.Create(r.Context(), fsmLayoutEntityType, map[string]any{
		"definition_id": id,
		"layout":        layout,
	})
	if err != nil {
		writeUpstreamErrorJSON(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
