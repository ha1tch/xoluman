// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ha1tch/xoluman/internal/connstore"
)

func newTestStore(t *testing.T) connstore.Store {
	t.Helper()
	return connstore.NewFileBackend(t.TempDir())
}

func TestConnectionsHandler_List_EmptyState(t *testing.T) {
	h := NewConnectionsHandler(newTestStore(t))
	req := httptest.NewRequest(http.MethodGet, "/connections", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "No connections yet") {
		t.Fatalf("body does not contain empty-state text: %s", rec.Body.String())
	}
}

func TestConnectionsHandler_NewForm_RendersFields(t *testing.T) {
	h := NewConnectionsHandler(newTestStore(t))
	req := httptest.NewRequest(http.MethodGet, "/connections/new", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	h.NewForm(rec, req)

	body := rec.Body.String()
	for _, want := range []string{`name="name"`, `name="base_url"`, `name="auth_mode"`, `name="token"`, `name="tenant"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing field %q: %s", want, body)
		}
	}
}

func TestConnectionsHandler_Create_ThenListedInTable(t *testing.T) {
	store := newTestStore(t)
	h := NewConnectionsHandler(store)

	form := url.Values{
		"name":      {"local"},
		"base_url":  {"http://localhost:8080"},
		"auth_mode": {"apikey"},
		"token":     {"secret"},
		"tenant":    {""},
	}
	req := httptest.NewRequest(http.MethodPost, "/connections", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Create status = %d, want %d (redirect)", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/connections" {
		t.Fatalf("Location = %q, want %q", loc, "/connections")
	}

	// Verify it was actually saved, not just that the handler redirected.
	got, err := store.Get(context.Background(), "local")
	if err != nil {
		t.Fatalf("store.Get after Create: %v", err)
	}
	if got.BaseURL != "http://localhost:8080" || got.Token != "secret" {
		t.Fatalf("saved connection = %+v, fields don't match submitted form", got)
	}

	// And that it renders in the list.
	listReq := httptest.NewRequest(http.MethodGet, "/connections", nil)
	listRec := httptest.NewRecorder()
	h.List(listRec, listReq)
	if !strings.Contains(listRec.Body.String(), "local") {
		t.Fatalf("list body does not contain the new connection's name: %s", listRec.Body.String())
	}
}

func TestConnectionsHandler_Create_ValidationFailureRedisplaysForm(t *testing.T) {
	h := NewConnectionsHandler(newTestStore(t))

	// Empty name: ErrInvalidName.
	form := url.Values{"name": {""}, "base_url": {"http://x"}, "auth_mode": {"none"}}
	req := httptest.NewRequest(http.MethodPost, "/connections", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if !strings.Contains(rec.Body.String(), "Name is required") {
		t.Fatalf("body does not contain the validation message: %s", rec.Body.String())
	}
	// The submitted base_url should still be in the redisplayed form —
	// losing what the person already typed would be a real usability bug.
	if !strings.Contains(rec.Body.String(), "http://x") {
		t.Fatalf("redisplayed form lost the submitted base_url: %s", rec.Body.String())
	}
}

func TestConnectionsHandler_Create_InvalidAuthModeRedisplaysForm(t *testing.T) {
	h := NewConnectionsHandler(newTestStore(t))

	form := url.Values{"name": {"local"}, "base_url": {"http://x"}, "auth_mode": {"not-a-real-mode"}}
	req := httptest.NewRequest(http.MethodPost, "/connections", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if !strings.Contains(rec.Body.String(), "Choose a valid auth mode") {
		t.Fatalf("body does not contain the auth-mode validation message: %s", rec.Body.String())
	}
}

func TestConnectionsHandler_DeleteConfirm_HTMXReturnsFragment(t *testing.T) {
	h := NewConnectionsHandler(newTestStore(t))
	req := httptest.NewRequest(http.MethodGet, "/connections/local/delete-confirm", nil)
	req.SetPathValue("name", "local")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	h.DeleteConfirm(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "local") || !strings.Contains(body, "/connections/local/delete") {
		t.Fatalf("body = %q, want the confirmation fragment for 'local'", body)
	}
	if strings.Contains(body, "<!DOCTYPE") {
		t.Fatalf("body = %q, want a bare fragment for an htmx request, not a full page", body)
	}
}

func TestConnectionsHandler_DeleteConfirm_DirectNavigationReturnsFullPage(t *testing.T) {
	h := NewConnectionsHandler(newTestStore(t))
	req := httptest.NewRequest(http.MethodGet, "/connections/local/delete-confirm", nil)
	req.SetPathValue("name", "local")
	rec := httptest.NewRecorder()

	h.DeleteConfirm(rec, req)

	if !strings.Contains(rec.Body.String(), "<!DOCTYPE") {
		t.Fatalf("body = %q, want the full page shell for a direct navigation", rec.Body.String())
	}
}

func TestConnectionsHandler_Delete_RemovesConnection(t *testing.T) {
	store := newTestStore(t)
	h := NewConnectionsHandler(store)
	ctx := context.Background()

	if _, err := store.Save(ctx, connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{Name: "local", BaseURL: "http://x", AuthMode: connstore.AuthNone},
	}); err != nil {
		t.Fatalf("seeding store: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/connections/local/delete", nil)
	req.SetPathValue("name", "local")
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if _, err := store.Get(ctx, "local"); err == nil {
		t.Fatal("connection still present after Delete")
	}
}

func TestConnectionsHandler_Delete_AlreadyGoneStillRedirects(t *testing.T) {
	// Deleting a connection that doesn't exist should behave the same as
	// deleting one that does, from the caller's perspective — the end
	// state (gone) is what they wanted either way.
	h := NewConnectionsHandler(newTestStore(t))
	req := httptest.NewRequest(http.MethodPost, "/connections/missing/delete", nil)
	req.SetPathValue("name", "missing")
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
}

func TestConnectionsHandler_Test_ReachableAgainstRealServer(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.Save(ctx, connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{Name: "up", BaseURL: upstream.URL, AuthMode: connstore.AuthNone},
	}); err != nil {
		t.Fatalf("seeding store: %v", err)
	}

	h := NewConnectionsHandler(store)
	req := httptest.NewRequest(http.MethodPost, "/connections/up/test", nil)
	req.SetPathValue("name", "up")
	rec := httptest.NewRecorder()

	h.Test(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "reachable") || strings.Contains(body, "unreachable") {
		t.Fatalf("body = %q, want a reachable (not unreachable) fragment", body)
	}
	if !strings.Contains(body, "text-green") {
		t.Fatalf("body = %q, want the reachable state styled distinctly from unreachable", body)
	}
}

func TestConnectionsHandler_Test_UnreachableWhenNothingListening(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	// Port 1 is reserved and nothing will ever be listening there.
	if _, err := store.Save(ctx, connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{Name: "down", BaseURL: "http://127.0.0.1:1", AuthMode: connstore.AuthNone},
	}); err != nil {
		t.Fatalf("seeding store: %v", err)
	}

	h := NewConnectionsHandler(store)
	req := httptest.NewRequest(http.MethodPost, "/connections/down/test", nil)
	req.SetPathValue("name", "down")
	rec := httptest.NewRecorder()

	h.Test(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "unreachable") {
		t.Fatalf("body = %q, want an unreachable fragment", body)
	}
	if !strings.Contains(body, "text-red") {
		t.Fatalf("body = %q, want the unreachable state styled distinctly from reachable", body)
	}
}

func TestConnectionsHandler_Test_UnknownConnectionName(t *testing.T) {
	h := NewConnectionsHandler(newTestStore(t))
	req := httptest.NewRequest(http.MethodPost, "/connections/nope/test", nil)
	req.SetPathValue("name", "nope")
	rec := httptest.NewRecorder()

	h.Test(rec, req)

	if !strings.Contains(rec.Body.String(), "not found") {
		t.Fatalf("body = %q, want a not-found fragment", rec.Body.String())
	}
}
