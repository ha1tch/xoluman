// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	xclient "github.com/ha1tch/xolu/pkg/client"
)

// clientForTestServer builds a bare xolu client pointed at a test
// server directly, bypassing the connection store entirely -- the
// resolver functions under test here take a *xclient.Client, not a
// connection name, so there's no reason to seed a stored connection
// just to unwrap it again.
func clientForTestServer(t *testing.T, baseURL string) *xclient.Client {
	t.Helper()
	return xclient.New(baseURL, xclient.WithTenant("acme"))
}

func TestCloseDealWonResolver_DerivesFullBindingsFromDealID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/tenant/acme/deals/6", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": 6, "name": "Big Deal", "amount": "5000.66",
			"primary_contact": {"entity":"contacts","id":17,"type":"REF"},
			"owner": {"entity":"users","id":4,"type":"REF"}
		}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := clientForTestServer(t, server.URL)

	bindings, err := closeDealWonResolver(context.Background(), c, map[string]string{
		"deal_id": "6", "close_notes": "custom note",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bindings["deal_id"] != int64(6) {
		t.Errorf("deal_id = %v, want 6", bindings["deal_id"])
	}
	if bindings["amount"] != "5000.66" {
		t.Errorf("amount = %v, want the deal's own amount (5000.66)", bindings["amount"])
	}
	if bindings["close_notes"] != "custom note" {
		t.Errorf("close_notes = %v, want the supplied note", bindings["close_notes"])
	}
	contactRef, ok := bindings["contact_ref"].(map[string]any)
	if !ok || contactRef["id"] != float64(17) {
		t.Errorf("contact_ref = %+v, want the deal's own primary_contact (contacts:17)", bindings["contact_ref"])
	}
	subject, _ := bindings["close_subject"].(string)
	if !strings.Contains(subject, "Big Deal") {
		t.Errorf("close_subject = %q, want it to mention the deal's own name", subject)
	}
}

func TestCloseDealWonResolver_DefaultsNotesWhenNotSupplied(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/tenant/acme/deals/6", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": 6, "name": "Big Deal", "amount": "5000.66"}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := clientForTestServer(t, server.URL)

	bindings, err := closeDealWonResolver(context.Background(), c, map[string]string{"deal_id": "6"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bindings["close_notes"] == "" {
		t.Error("close_notes should default to something, not be left empty")
	}
}

func TestCloseDealWonResolver_MissingDealIDIsRejected(t *testing.T) {
	c := clientForTestServer(t, "http://unused.invalid")
	_, err := closeDealWonResolver(context.Background(), c, map[string]string{})
	if err == nil {
		t.Fatal("expected an error for a missing deal_id, got nil")
	}
}

func TestCloseDealWonResolver_NonNumericDealIDIsRejected(t *testing.T) {
	c := clientForTestServer(t, "http://unused.invalid")
	_, err := closeDealWonResolver(context.Background(), c, map[string]string{"deal_id": "not-a-number"})
	if err == nil {
		t.Fatal("expected an error for a non-numeric deal_id, got nil")
	}
}

func TestCloseDealWonResolver_DealWithNoAmountIsRejected(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/tenant/acme/deals/6", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": 6, "name": "Big Deal"}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := clientForTestServer(t, server.URL)

	_, err := closeDealWonResolver(context.Background(), c, map[string]string{"deal_id": "6"})
	if err == nil {
		t.Fatal("expected an error for a deal with no amount set, got nil")
	}
}

func TestMarkDealLostResolver_RequiresAReason(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/tenant/acme/deals/6", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": 6, "name": "Big Deal"}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := clientForTestServer(t, server.URL)

	_, err := markDealLostResolver(context.Background(), c, map[string]string{"deal_id": "6"})
	if err == nil {
		t.Fatal("expected an error for a missing lost_reason, got nil")
	}
}

func TestMarkDealLostResolver_DerivesBindingsFromDealID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/tenant/acme/deals/6", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": 6, "name": "Big Deal",
			"primary_contact": {"entity":"contacts","id":17,"type":"REF"},
			"owner": {"entity":"users","id":4,"type":"REF"}
		}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := clientForTestServer(t, server.URL)

	bindings, err := markDealLostResolver(context.Background(), c, map[string]string{
		"deal_id": "6", "lost_reason": "went with a competitor",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bindings["lost_reason"] != "went with a competitor" {
		t.Errorf("lost_reason = %v, want the supplied reason", bindings["lost_reason"])
	}
	if _, hasBal := bindings["amount"]; hasBal {
		t.Error("mark_deal_lost should have no amount/bal binding at all -- a lost deal recognizes no revenue")
	}
	title, _ := bindings["followup_title"].(string)
	if !strings.Contains(title, "Big Deal") {
		t.Errorf("followup_title = %q, want it to mention the deal's own name", title)
	}
	if bindings["followup_due_date"] == "" || bindings["followup_due_date"] == nil {
		t.Error("followup_due_date should be set -- the third DXP participant (the follow-up task) requires it")
	}
}

func TestRunDXPPreset_UnknownResolverRejected(t *testing.T) {
	store := seedConnection(t, "http://unused.invalid")
	h := &queryHandler{store: store}

	body, _ := json.Marshal(dxpPresetRunRequest{Resolver: "nonexistent_resolver", DefName: "x", FormValues: map[string]string{}})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/dxp/preset-run", bytes.NewReader(body))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.RunDXPPreset(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d for an unknown resolver name", rec.Code, http.StatusBadRequest)
	}
}

func TestRunDXPPreset_UnknownDefNameRejected(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/deals/6", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": 6, "name": "Big Deal", "amount": "100.00"}`))
	})
	mux.HandleFunc("/api/v2/dxp/def", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"definitions":[{"id":1,"name":"some_other_def"}]}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &queryHandler{store: store}

	body, _ := json.Marshal(dxpPresetRunRequest{
		Resolver: "close_deal_won", DefName: "close_deal_won",
		FormValues: map[string]string{"deal_id": "6"},
	})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/dxp/preset-run", bytes.NewReader(body))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.RunDXPPreset(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d for a def name not registered on this connection; body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestRunDXPPreset_ResolverErrorSurfacedAsUnprocessable(t *testing.T) {
	store := seedConnection(t, "http://unused.invalid")
	h := &queryHandler{store: store}

	body, _ := json.Marshal(dxpPresetRunRequest{
		Resolver: "close_deal_won", DefName: "close_deal_won",
		FormValues: map[string]string{}, // missing deal_id
	})
	req := httptest.NewRequest(http.MethodPost, "/connections/test/dxp/preset-run", bytes.NewReader(body))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.RunDXPPreset(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d for a resolver validation failure", rec.Code, http.StatusUnprocessableEntity)
	}
}
