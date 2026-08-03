// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ha1tch/xoluman/internal/connstore"
)

func TestServer_StaticAssets_ServesTailwindCSS(t *testing.T) {
	h := New(connstore.NewFileBackend(t.TempDir()))
	req := httptest.NewRequest(http.MethodGet, "/static/css/tailwind.css", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d — did you run `npm run css`?", rec.Code, http.StatusOK)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("tailwind.css is empty")
	}
}

func TestServer_StaticAssets_ServesVendoredHtmx(t *testing.T) {
	h := New(connstore.NewFileBackend(t.TempDir()))
	req := httptest.NewRequest(http.MethodGet, "/static/vendor/htmx@1.9.10.min.js", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "htmx") {
		t.Fatalf("body does not look like htmx: first 100 bytes = %q", rec.Body.String()[:100])
	}
}

func TestServer_StaticAssets_ServesEmbeddedModalJS(t *testing.T) {
	h := New(connstore.NewFileBackend(t.TempDir()))
	req := httptest.NewRequest(http.MethodGet, "/static/js/modal.js", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "window.XModal") {
		t.Fatalf("body does not look like modal.js: %s", rec.Body.String())
	}
}

func TestServer_DeleteConfirmRouteExtractsPathValue(t *testing.T) {
	store := connstore.NewFileBackend(t.TempDir())
	if _, err := store.Save(httptest.NewRequest(http.MethodGet, "/", nil).Context(), connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{Name: "local", BaseURL: "http://x", AuthMode: connstore.AuthNone},
	}); err != nil {
		t.Fatalf("seeding store: %v", err)
	}

	h := New(store)
	req := httptest.NewRequest(http.MethodGet, "/connections/local/delete-confirm", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "local") {
		t.Fatalf("body doesn't mention the connection name: %s", rec.Body.String())
	}
}

func TestServer_RootRedirectsToConnections(t *testing.T) {
	h := New(connstore.NewFileBackend(t.TempDir()))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/connections" {
		t.Fatalf("Location = %q, want %q", loc, "/connections")
	}
}

func TestServer_ConnectionsListReachableThroughMux(t *testing.T) {
	h := New(connstore.NewFileBackend(t.TempDir()))
	req := httptest.NewRequest(http.MethodGet, "/connections", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "Connections") {
		t.Fatalf("body doesn't look like the connections page: %s", rec.Body.String())
	}
}

func TestServer_DeleteRouteExtractsPathValue(t *testing.T) {
	store := connstore.NewFileBackend(t.TempDir())
	if _, err := store.Save(httptest.NewRequest(http.MethodGet, "/", nil).Context(), connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{Name: "local", BaseURL: "http://x", AuthMode: connstore.AuthNone},
	}); err != nil {
		t.Fatalf("seeding store: %v", err)
	}

	h := New(store)
	req := httptest.NewRequest(http.MethodPost, "/connections/local/delete", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if _, err := store.Get(req.Context(), "local"); err == nil {
		t.Fatal("connection still present — {name} path value was not routed through correctly")
	}
}
