// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	xclient "github.com/ha1tch/xolu/pkg/client"
)

func TestExtractBindingNames_TopLevelRef(t *testing.T) {
	participants := []xclient.DxpParticipant{
		{ID: "p1", Params: map[string]interface{}{"id": map[string]interface{}{"$ref": "user_id"}}},
	}
	got := extractBindingNames(participants)
	want := []string{"user_id"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExtractBindingNames_NestedInObject(t *testing.T) {
	participants := []xclient.DxpParticipant{
		{ID: "p1", Params: map[string]interface{}{
			"filter": map[string]interface{}{
				"status": map[string]interface{}{"$ref": "target_status"},
			},
		}},
	}
	got := extractBindingNames(participants)
	want := []string{"target_status"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v — nested-in-object $ref not found", got, want)
	}
}

func TestExtractBindingNames_NestedInArray(t *testing.T) {
	participants := []xclient.DxpParticipant{
		{ID: "p1", Params: map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"$ref": "first_item"},
				map[string]interface{}{"$ref": "second_item"},
				"literal-value",
			},
		}},
	}
	got := extractBindingNames(participants)
	want := []string{"first_item", "second_item"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v — nested-in-array $ref not found", got, want)
	}
}

func TestExtractBindingNames_DeduplicatesAcrossParticipants(t *testing.T) {
	participants := []xclient.DxpParticipant{
		{ID: "p1", Params: map[string]interface{}{"id": map[string]interface{}{"$ref": "shared_id"}}},
		{ID: "p2", Params: map[string]interface{}{"ref_id": map[string]interface{}{"$ref": "shared_id"}}},
	}
	got := extractBindingNames(participants)
	want := []string{"shared_id"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v — same binding referenced by two participants should appear once", got, want)
	}
}

func TestExtractBindingNames_SortedOrder(t *testing.T) {
	participants := []xclient.DxpParticipant{
		{ID: "p1", Params: map[string]interface{}{
			"z": map[string]interface{}{"$ref": "zebra"},
			"a": map[string]interface{}{"$ref": "apple"},
			"m": map[string]interface{}{"$ref": "mango"},
		}},
	}
	got := extractBindingNames(participants)
	want := []string{"apple", "mango", "zebra"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v — must be sorted for a stable form field order", got, want)
	}
}

func TestExtractBindingNames_LiteralParamsIgnored(t *testing.T) {
	// A literal value (no $ref) is a fixed constant the def's author
	// chose — not something a caller fills in, so it must not appear
	// as a binding to prompt for.
	participants := []xclient.DxpParticipant{
		{ID: "p1", Params: map[string]interface{}{
			"amount": 42.0,
			"active": true,
			"label":  "fixed-string",
		}},
	}
	got := extractBindingNames(participants)
	if len(got) != 0 {
		t.Fatalf("got %v, want none — no $ref anywhere in these params", got)
	}
}

func TestExtractBindingNames_ObjectWithDollarRefPlusOtherKeysIsNotARef(t *testing.T) {
	// {"$ref": "x", "extra": 1} is NOT the documented ref shape (which
	// is exactly one key) — real object data that happens to contain a
	// key literally named "$ref" alongside other data should not be
	// misread as a binding placeholder.
	participants := []xclient.DxpParticipant{
		{ID: "p1", Params: map[string]interface{}{
			"weird": map[string]interface{}{"$ref": "not_a_real_binding", "other": "field"},
		}},
	}
	got := extractBindingNames(participants)
	if len(got) != 0 {
		t.Fatalf("got %v, want none — a multi-key object containing \"$ref\" is not the ref shape", got)
	}
}

func TestDXPHandler_ListDefs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v2/dxp/def", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"definitions": []map[string]any{
				{"id": float64(2), "name": "Zeta transfer", "created_at": "2026-08-01T00:00:00Z"},
				{"id": float64(1), "name": "Alpha transfer", "created_at": "2026-08-01T00:00:00Z"},
			},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &dxpHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/dxp/defs", nil)
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()
	h.ListDefs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []dxpDefSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if len(got) != 2 || got[0].Name != "Alpha transfer" || got[1].Name != "Zeta transfer" {
		t.Fatalf("got %+v, want alphabetically sorted by name", got)
	}
}

func TestDXPHandler_GetDef_IncludesExtractedBindings(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v2/dxp/def/1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         float64(1),
			"name":       "Transfer funds",
			"created_at": "2026-08-01T00:00:00Z",
			"spec": map[string]any{
				"name":    "Transfer funds",
				"pattern": "2pc",
				"participants": []map[string]any{
					{
						"id": "debit", "primitive": "bal", "op": "debit",
						"params": map[string]any{
							"account": map[string]any{"$ref": "from_account"},
							"amount":  map[string]any{"$ref": "amount"},
						},
					},
					{
						"id": "credit", "primitive": "bal", "op": "credit",
						"params": map[string]any{
							"account": map[string]any{"$ref": "to_account"},
							"amount":  map[string]any{"$ref": "amount"},
						},
					},
				},
				"phase_ttl": map[string]any{"reserve": "30s"},
			},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &dxpHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/dxp/defs/1", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	h.GetDef(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got dxpDefDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	want := []string{"amount", "from_account", "to_account"}
	if !reflect.DeepEqual(got.Bindings, want) {
		t.Fatalf("bindings = %v, want %v (deduplicated across both participants, sorted)", got.Bindings, want)
	}
	if len(got.Participants) != 2 {
		t.Fatalf("participants = %d, want 2", len(got.Participants))
	}
}

func TestDXPHandler_GetDef_InvalidID(t *testing.T) {
	h := &dxpHandler{store: newTestStore(t)}
	req := httptest.NewRequest(http.MethodGet, "/connections/test/dxp/defs/notanumber", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("id", "notanumber")
	rec := httptest.NewRecorder()
	h.GetDef(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDXPHandler_Run_CommittedOutcome(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/dxp/txn", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": float64(9), "def_id": float64(1), "status": "committed",
			"committed_through": float64(2), "created_at": "2026-08-04T00:00:00Z",
			"snapshot": map[string]any{"pattern": "2pc", "participants": []any{}, "phase_ttl": map[string]any{"reserve": "30s"}},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &dxpHandler{store: store}

	body, _ := json.Marshal(dxpRunRequest{DefID: 1, Bindings: map[string]interface{}{"amount": 100.0, "from_account": "A"}})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/dxp/run", strings.NewReader(string(body)))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()
	h.Run(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotBody["def_id"] != float64(1) {
		t.Fatalf("upstream request def_id = %v, want 1", gotBody["def_id"])
	}
	var got xclient.DxpTxn
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if got.Status != "committed" {
		t.Fatalf("status = %q, want %q", got.Status, "committed")
	}
}

func TestDXPHandler_Run_ReleasedOutcomeIsNotAnHTTPError(t *testing.T) {
	// Confirmed directly against DxpTxnCreate's own doc comment: a
	// non-committed outcome is a normal response, not an error — the
	// handler must return 200 with the real status, not surface this
	// as a failure the way writeUpstreamError would.
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/dxp/txn", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": float64(10), "def_id": float64(1), "status": "released", "reason": "participant declined",
			"committed_through": float64(0), "created_at": "2026-08-04T00:00:00Z",
			"snapshot": map[string]any{"pattern": "2pc", "participants": []any{}, "phase_ttl": map[string]any{"reserve": "30s"}},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &dxpHandler{store: store}

	body, _ := json.Marshal(dxpRunRequest{DefID: 1, Bindings: map[string]interface{}{}})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/dxp/run", strings.NewReader(string(body)))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()
	h.Run(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (a released outcome is not an HTTP error)", rec.Code, http.StatusOK)
	}
	var got xclient.DxpTxn
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if got.Status != "released" || got.Reason != "participant declined" {
		t.Fatalf("status/reason = %q/%q, want released/participant declined", got.Status, got.Reason)
	}
}

func TestDXPHandler_Run_InvalidDefIDRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(server.Close)
	h := &dxpHandler{store: seedConnection(t, server.URL)}
	body, _ := json.Marshal(dxpRunRequest{DefID: 0})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/dxp/run", strings.NewReader(string(body)))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()
	h.Run(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
