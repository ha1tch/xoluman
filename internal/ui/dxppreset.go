// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// dxppreset.go — saved DXP invocations ("DXP presets"): a named DXP
// def paired with a small, human-facing form and a resolver that
// turns the form's simple values into the full binding set the def
// actually needs.
//
// This exists because a DXP def's own bindings are frequently not
// form-friendly on their own — close_deal_won (see the CRM seed
// script) needs deal_id, deal_ref, contact_ref, owner_ref,
// close_subject, close_date, close_notes, and amount, but a person
// closing a deal shouldn't have to type out REF objects for the
// contact and owner by hand. They should pick a deal and, at most,
// add a note — everything else is already on the deal record.
//
// A fully generic version of this (inspect any DXP def's own
// bindings, build a form automatically, resolve $ref-shaped bindings
// against arbitrary entity lookups) would be a genuinely large
// feature — introspecting arbitrary binding semantics without any
// declared schema for what a binding *means* (is "amount" a literal,
// or does it come from somewhere?) isn't something the def itself
// currently expresses. What's here instead is a small, honest,
// hand-written registry: each preset's resolver is real Go code that
// knows specifically how to build one specific def's bindings. Adding
// a new preset means adding a new resolver function, not writing
// configuration that pretends to be more generic than it is.
package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/connstore"
	"github.com/ha1tch/xoluman/internal/xoluext"
)

// dxpPresetFormField describes one input the person fills in — the
// UI renders these directly, the resolver reads the submitted values
// back by Key.
type dxpPresetFormField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
}

// dxpPreset pairs a resolver name (looked up in dxpPresetResolvers)
// with the def it targets and the form describing what a person needs
// to actually supply. Stored data (seeded via xoluman_saved_query,
// mode: "dxp") carries DefName/Resolver/FormFields; the resolver
// function itself is never stored — only ever looked up by name from
// the in-code registry below, so a saved preset can't reference code
// that doesn't exist.
type dxpPreset struct {
	Mode       string               `json:"mode"`
	Name       string               `json:"name"`
	DefName    string               `json:"dxp_def_name"`
	Resolver   string               `json:"resolver"`
	FormFields []dxpPresetFormField `json:"form_fields"`
}

// dxpPresetResolver turns a form's raw submitted values into the full
// binding set its target def needs, looking up whatever related data
// it requires along the way. formValues are always plain strings —
// exactly what an HTML form actually submits — so a resolver that
// needs a number parses it itself and returns a clear error if the
// person typed something else.
type dxpPresetResolver func(ctx context.Context, c *xclient.Client, formValues map[string]string) (map[string]any, error)

// closeDealWonResolver mirrors exactly what the CRM seed script's own
// close_deal_won invocation already does by hand (see
// examples/crm/xolu_crm_seed.py) — look up the deal, derive every
// other binding from its own fields, so the person filling in the
// form only ever supplies the deal and, optionally, a note.
func closeDealWonResolver(ctx context.Context, c *xclient.Client, formValues map[string]string) (map[string]any, error) {
	dealID, err := parseFormInt(formValues, "deal_id")
	if err != nil {
		return nil, err
	}
	deal, err := c.Get(ctx, "deals", dealID)
	if err != nil {
		return nil, fmt.Errorf("looking up deal %d: %w", dealID, err)
	}
	amount, ok := deal.Data["amount"]
	if !ok {
		return nil, fmt.Errorf("deal %d has no amount set — close_deal_won needs one to recognize revenue", dealID)
	}
	name, _ := deal.Data["name"].(string)

	notes := formValues["close_notes"]
	if notes == "" {
		notes = "Closed via the graph viewer's close-deal preset."
	}

	return map[string]any{
		"deal_id":       dealID,
		"deal_ref":      map[string]any{"entity": "deals", "id": dealID, "type": "REF"},
		"contact_ref":   deal.Data["primary_contact"],
		"owner_ref":     deal.Data["owner"],
		"close_subject": fmt.Sprintf("Closed won: %s", name),
		"close_date":    time.Now().UTC().Format(time.RFC3339),
		"close_notes":   notes,
		"amount":        amount,
	}, nil
}

// markDealLostResolver is the mirror-image preset for the other side
// of deal_lifecycle's own closed_lost transitions — same lookup-the-
// deal shape as closeDealWonResolver, but logging why it was lost
// instead of recognizing revenue. The third participant (a follow-up
// debrief task) exists because xolu currently only implements the
// "3ps" DXP pattern (2ps and wider are deferred — confirmed directly
// by trying 2ps first and reading the real rejection, XOLU-DXP006,
// not assumed) — a genuine third step was needed, so this adds one
// that's actually useful rather than padding: prompting the rep to
// debrief on a lost deal is a real CRM pattern, not an artificial fit
// to the pattern requirement.
func markDealLostResolver(ctx context.Context, c *xclient.Client, formValues map[string]string) (map[string]any, error) {
	dealID, err := parseFormInt(formValues, "deal_id")
	if err != nil {
		return nil, err
	}
	deal, err := c.Get(ctx, "deals", dealID)
	if err != nil {
		return nil, fmt.Errorf("looking up deal %d: %w", dealID, err)
	}
	name, _ := deal.Data["name"].(string)

	reason := formValues["lost_reason"]
	if reason == "" {
		return nil, errors.New("lost_reason is required — recording why a deal was lost is the whole point of this transaction")
	}

	return map[string]any{
		"deal_id":           dealID,
		"deal_ref":          map[string]any{"entity": "deals", "id": dealID, "type": "REF"},
		"contact_ref":       deal.Data["primary_contact"],
		"owner_ref":         deal.Data["owner"],
		"lost_subject":      fmt.Sprintf("Closed lost: %s", name),
		"lost_date":         time.Now().UTC().Format(time.RFC3339),
		"lost_reason":       reason,
		"followup_title":    fmt.Sprintf("Debrief: why we lost %s", name),
		"followup_due_date": time.Now().UTC().AddDate(0, 0, 3).Format("2006-01-02"),
	}, nil
}

var dxpPresetResolvers = map[string]dxpPresetResolver{
	"close_deal_won": closeDealWonResolver,
	"mark_deal_lost": markDealLostResolver,
}

func parseFormInt(formValues map[string]string, key string) (int64, error) {
	raw, ok := formValues[key]
	if !ok || raw == "" {
		return 0, fmt.Errorf("%s is required", key)
	}
	var id int64
	if _, err := fmt.Sscanf(raw, "%d", &id); err != nil || id <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer, got %q", key, raw)
	}
	return id, nil
}

type dxpPresetRunRequest struct {
	Resolver   string            `json:"resolver"`
	DefName    string            `json:"defName"`
	FormValues map[string]string `json:"formValues"`
}

// RunDXPPreset resolves a preset's form values into a full binding
// set, looks up its def by name (the same DxpDefList xoluman's own
// DXP module already uses — dxp.go's ListDefs), then runs it through
// the identical DxpTxnCreate call dxp.go's own Run handler uses. No
// new way of actually executing a DXP transaction is introduced here
// — only a new way of building the bindings for one.
func (h *queryHandler) RunDXPPreset(w http.ResponseWriter, r *http.Request) {
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

	var req dxpPresetRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	resolver, ok := dxpPresetResolvers[req.Resolver]
	if !ok {
		http.Error(w, fmt.Sprintf("unknown resolver %q", req.Resolver), http.StatusBadRequest)
		return
	}

	bindings, err := resolver(r.Context(), c, req.FormValues)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	defList, err := c.DxpDefList(r.Context())
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}
	var defID int64
	for _, d := range defList.Definitions {
		if d.Name == req.DefName {
			defID = d.ID
			break
		}
	}
	if defID == 0 {
		http.Error(w, fmt.Sprintf("no DXP def named %q is registered on this connection", req.DefName), http.StatusUnprocessableEntity)
		return
	}

	txn, err := c.DxpTxnCreate(r.Context(), xclient.DxpTxnCreateRequest{DefID: defID, Bindings: bindings})
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(txn)
}
