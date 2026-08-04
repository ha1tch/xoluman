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
		// expects (mirrors connections.go's own init() — Label is
		// deliberately empty, matching the real registration: the
		// brand/home link in PageWithHead covers this now, not a
		// separate always-on nav item).
		registry = modules.NewRegistry()
		registry.Register(modules.Module{ID: "connections", Label: "", URL: "/connections", ActivePrefix: "/connections", Order: 0})
	}()

	html := mi.RenderToString(Page("Test", "/", func(b *mi.Builder) mi.Node { return b.P("body") }))
	if !strings.Contains(html, "TestOnlyModule") {
		t.Fatalf("nav doesn't reflect a freshly-registered module — nav must be registry-driven, not hardcoded: %s", html)
	}
}

func TestPage_NavMarksActiveModuleByPrefix(t *testing.T) {
	// Connections itself no longer has a nav Label to mark active (the
	// brand/home link replaced it — see PageWithHead's own doc
	// comment on why a permanently-active nav item was the actual
	// design problem being fixed). Register a genuine Label-bearing
	// module here instead, matching what any future nav-visible module
	// would look like, and prove active-marking still works correctly
	// for it.
	registry.Register(modules.Module{ID: "active-test-module", Label: "ActiveTestModule", URL: "/active-test", ActivePrefix: "/active-test", Order: 999})
	defer func() {
		registry = modules.NewRegistry()
		registry.Register(modules.Module{ID: "connections", Label: "", URL: "/connections", ActivePrefix: "/connections", Order: 0})
	}()

	activeHTML := mi.RenderToString(Page("Test", "/active-test/sub-path", func(b *mi.Builder) mi.Node { return b.P("body") }))
	if !strings.Contains(activeHTML, `href="/active-test"`) || !strings.Contains(activeHTML, `class="text-gray-900 dark:text-white font-semibold no-underline" href="/active-test"`) {
		t.Fatalf("ActiveTestModule not marked active for a matching path: %s", activeHTML)
	}

	inactiveHTML := mi.RenderToString(Page("Test", "/connections", func(b *mi.Builder) mi.Node { return b.P("body") }))
	if !strings.Contains(inactiveHTML, `class="text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white no-underline" href="/active-test"`) {
		t.Fatalf("ActiveTestModule marked active for a non-matching path: %s", inactiveHTML)
	}
}

func TestPage_BrandLinkIsHomeAndAlwaysPresent(t *testing.T) {
	// The brand mark is the home link now, not a nav "tab" that
	// pretends to have active/inactive states it never meaningfully
	// had (see PageWithHead's doc comment) — it should render
	// identically regardless of which page it's on.
	for _, path := range []string{"/connections", "/connections/local/entities/widgets", "/connections/local/blobs/photos"} {
		html := mi.RenderToString(Page("Test", path, func(b *mi.Builder) mi.Node { return b.P("body") }))
		if !strings.Contains(html, `class="font-semibold text-gray-900 dark:text-white no-underline hover:text-indigo-600 dark:hover:text-indigo-400" href="/connections">xoluman<`) {
			t.Fatalf("brand/home link missing or changed for path %q: %s", path, html)
		}
	}
}

func TestPage_ThemeToggleAndThemeJSPresent(t *testing.T) {
	html := mi.RenderToString(Page("Test", "/connections", func(b *mi.Builder) mi.Node { return b.P("body") }))
	if !strings.Contains(html, `id="theme-toggle-btn"`) {
		t.Fatalf("theme toggle button missing: %s", html)
	}
	if !strings.Contains(html, `onclick="xoluTheme.toggle()"`) {
		t.Fatalf("theme toggle button missing its onclick handler: %s", html)
	}
	if !strings.Contains(html, `src="/static/js/theme.js"`) {
		t.Fatalf("theme.js not loaded: %s", html)
	}
	// theme.js must load before the stylesheet (and everything else)
	// to set the dark/light class before first paint — this ordering
	// is the entire point, not incidental.
	themeIdx := strings.Index(html, "theme.js")
	cssIdx := strings.Index(html, "tailwind.css")
	if themeIdx == -1 || cssIdx == -1 || themeIdx > cssIdx {
		t.Fatalf("theme.js must load before the stylesheet to avoid a flash of the wrong theme: %s", html)
	}
}
