// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mi "github.com/ha1tch/minty"

	"github.com/ha1tch/xoluman/internal/modules"
)

func TestTable_EmptyShowsEmptyMessage(t *testing.T) {
	html := mi.RenderToString(Table([]string{"A", "B"}, nil, "nothing here"))
	if !strings.Contains(html, "nothing here") {
		t.Fatalf("html = %q, want empty message", html)
	}
	if strings.Contains(html, "<table") {
		t.Fatalf("html = %q, want no <table> in the empty state", html)
	}
}

func TestTable_RendersColumnsAndRows(t *testing.T) {
	row := mi.B.Tr(mi.B.Td("cell-value"))
	html := mi.RenderToString(Table([]string{"Col A", "Col B"}, []mi.Node{row}, "unused"))

	for _, want := range []string{"Col A", "Col B", "cell-value"} {
		if !strings.Contains(html, want) {
			t.Fatalf("html = %q, want it to contain %q", html, want)
		}
	}
}

func TestModalTriggerButton_HasHxAndOnclick(t *testing.T) {
	html := mi.RenderToString(ModalTriggerButton("+ New thing", "New thing", "/things/new", "btn"))

	for _, want := range []string{
		`hx-get="/things/new"`,
		`hx-target="#modal-body"`,
		`onclick="XModal.open(&#39;New thing&#39;)"`,
		"+ New thing",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("html = %q, want it to contain %q", html, want)
		}
	}
}

func TestDeleteConfirmBody_HasFormAndCancel(t *testing.T) {
	html := mi.RenderToString(DeleteConfirmBody("widget-1", "/widgets/widget-1/delete"))

	if !strings.Contains(html, `action="/widgets/widget-1/delete"`) {
		t.Fatalf("html = %q, want the delete form action", html)
	}
	if !strings.Contains(html, "widget-1") {
		t.Fatalf("html = %q, want the item label", html)
	}
	if !strings.Contains(html, "XModal.close()") {
		t.Fatalf("html = %q, want a Cancel button that closes the modal", html)
	}
}

func TestWriteModalAware_HTMXRequest_WritesBareFragment(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	WriteModalAware(rec, req, "Title", func(b *mi.Builder) mi.Node { return b.P("fragment-content") })

	body := rec.Body.String()
	if !strings.Contains(body, "fragment-content") {
		t.Fatalf("body = %q, want the fragment content", body)
	}
	if strings.Contains(body, "<nav") || strings.Contains(body, "<!DOCTYPE") {
		t.Fatalf("body = %q, want no page shell for an htmx request", body)
	}
}

func TestWriteModalAware_DirectNavigation_WritesFullPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil) // no HX-Request header
	rec := httptest.NewRecorder()

	WriteModalAware(rec, req, "Title", func(b *mi.Builder) mi.Node { return b.P("fragment-content") })

	body := rec.Body.String()
	if !strings.Contains(body, "fragment-content") {
		t.Fatalf("body = %q, want the fragment content still present", body)
	}
	if !strings.Contains(body, "<nav") || !strings.Contains(body, "<!DOCTYPE") {
		t.Fatalf("body = %q, want the full page shell for a direct navigation", body)
	}
}

func TestPage_NoExternalCDNReferences(t *testing.T) {
	// xoluman must run fully disconnected — it manages xolu instances
	// that may themselves be on air-gapped or otherwise offline local
	// networks. Every asset the page shell references must be served
	// from /static/, not fetched from a CDN or any other live internet
	// dependency. See docs/KNOWN_ISSUES.md's recorded decision.
	html := mi.RenderToString(Page("Test", "/", func(b *mi.Builder) mi.Node { return b.P("body") }))

	for _, external := range []string{"http://", "https://", "//unpkg.com", "//cdn.", "//jsdelivr", "//cdnjs"} {
		if strings.Contains(html, external) {
			t.Fatalf("page shell contains an external reference (%q) — everything must be served from /static/: %s", external, html)
		}
	}
	if !strings.Contains(html, `src="/static/vendor/htmx`) {
		t.Fatalf("page shell doesn't reference vendored htmx under /static/vendor/: %s", html)
	}
}

func TestPage_NavReflectsRegisteredModules(t *testing.T) {
	// The nav must come from whatever's actually registered, not a
	// hardcoded list — proven here by registering a second module that
	// nothing in production code knows about, and confirming it
	// genuinely shows up, then removing it so this test doesn't leak
	// state into any other test.
	registry.Register(modules.Module{ID: "test-only-module", Label: "TestOnlyModule", URL: "/test-only", ActivePrefix: "/test-only", Order: 999})
	defer func() {
		// registry has no Remove; rebuild it from scratch minus the
		// probe, then re-register what production code actually
		// expects (mirrors connections.go's own init()).
		registry = modules.NewRegistry()
		registry.Register(modules.Module{ID: "connections", Label: "Connections", URL: "/connections", ActivePrefix: "/connections", Order: 0})
	}()

	html := mi.RenderToString(Page("Test", "/", func(b *mi.Builder) mi.Node { return b.P("body") }))
	if !strings.Contains(html, "TestOnlyModule") {
		t.Fatalf("nav doesn't reflect a freshly-registered module — nav must be registry-driven, not hardcoded: %s", html)
	}
}

func TestPage_NavMarksActiveModuleByPrefix(t *testing.T) {
	html := mi.RenderToString(Page("Test", "/connections/local/entities", func(b *mi.Builder) mi.Node { return b.P("body") }))
	if !strings.Contains(html, "font-semibold") {
		t.Fatalf("nav doesn't mark Connections active for a /connections/... path: %s", html)
	}
}
