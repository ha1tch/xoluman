// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mi "github.com/ha1tch/minty"

	"github.com/ha1tch/xoluman/internal/connstore"
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

func TestTable_HasElevatedWrapperAndStriping(t *testing.T) {
	// Real gap this guards against, reported directly: the table had
	// no wrapper styling at all — no shadow, no ring, no rounded
	// corners, no row striping — despite minty's own Tailwind theme
	// (themes/tailwind/tailwind.go's Table()) establishing exactly
	// this convention, which xoluman's implementation had never
	// actually adopted.
	row := mi.B.Tr(mi.B.Td("cell-value"))
	html := mi.RenderToString(Table([]string{"Col A"}, []mi.Node{row}, "unused"))

	for _, want := range []string{"shadow-sm", "ring-1", "rounded-lg", "nth-child(even)", "uppercase", "tracking-wider"} {
		if !strings.Contains(html, want) {
			t.Fatalf("html missing %q — the elevated-card wrapper/header/striping convention: %s", want, html)
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

func TestRefJumpButton_OpensLinkedEntityInModal(t *testing.T) {
	// Direct request: a small icon next to a ref link that opens the
	// linked entity in a modal instead of a full-page navigation away.
	html := mi.RenderToString(RefJumpButton("/connections/local/entities/users/5/edit"))

	for _, want := range []string{
		`hx-get="/connections/local/entities/users/5/edit"`,
		`hx-target="#modal-body"`,
		"<svg",
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

	// Checking for src="http.../href="http... specifically, not any
	// bare "http://" substring — minty's DarkMode SVG icons correctly
	// include xmlns="http://www.w3.org/2000/svg" (an XML namespace
	// identifier, not a network reference), which a naive substring
	// check flagged as a false positive.
	for _, external := range []string{`src="http`, `href="http`, `src='http`, `href='http`, "//unpkg.com", "//cdn.", "//jsdelivr", "//cdnjs"} {
		if strings.Contains(html, external) {
			t.Fatalf("page shell contains an external reference (%q) — everything must be served from /static/: %s", external, html)
		}
	}
	if !strings.Contains(html, `src="/static/vendor/htmx`) {
		t.Fatalf("page shell doesn't reference vendored htmx under /static/vendor/: %s", html)
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
	// theme.js (hand-rolled) was replaced with minty's own built-in
	// DarkMode mechanism (darkmode.go) — this checks for what that
	// actually generates, not the removed custom implementation.
	html := mi.RenderToString(Page("Test", "/connections", func(b *mi.Builder) mi.Node { return b.P("body") }))
	if !strings.Contains(html, `onclick="toggleDarkMode()"`) {
		t.Fatalf("theme toggle button missing its onclick handler: %s", html)
	}
	if !strings.Contains(html, `id="dark-mode-icon"`) {
		t.Fatalf("theme toggle icon element missing: %s", html)
	}
	if !strings.Contains(html, "function toggleDarkMode()") {
		t.Fatalf("minty's DarkMode init script not present: %s", html)
	}
	// The init script must run before the stylesheet (and everything
	// else) to set the dark/light class before first paint — this
	// ordering is the entire point, not incidental.
	scriptIdx := strings.Index(html, "function toggleDarkMode()")
	cssIdx := strings.Index(html, "tailwind.css")
	if scriptIdx == -1 || cssIdx == -1 || scriptIdx > cssIdx {
		t.Fatalf("DarkMode init script must load before the stylesheet to avoid a flash of the wrong theme: %s", html)
	}
}

func TestConnectionNameFromPath(t *testing.T) {
	cases := []struct {
		path     string
		wantName string
		wantOK   bool
	}{
		{"/connections", "", false},
		{"/connections/", "", false},
		{"/connections/new", "", false},
		{"/connections/local", "local", true},
		{"/connections/local/entities", "local", true},
		{"/connections/local/entities/widgets/1/edit", "local", true},
		{"/connections/Prod%20Server/entities", "Prod Server", true},
		{"/", "", false},
		{"/static/js/modal.js", "", false},
	}
	for _, c := range cases {
		name, ok := connectionNameFromPath(c.path)
		if ok != c.wantOK || name != c.wantName {
			t.Errorf("connectionNameFromPath(%q) = %q, %v, want %q, %v", c.path, name, ok, c.wantName, c.wantOK)
		}
	}
}

func TestPage_ConnectionNameCenteredWhenInsideAConnection(t *testing.T) {
	inside := mi.RenderToString(Page("Test", "/connections/local/entities", func(b *mi.Builder) mi.Node { return b.P("body") }))
	if !strings.Contains(inside, `class="justify-self-center"><div class="font-semibold text-gray-900 dark:text-white truncate max-w-xs">local</div>`) {
		t.Fatalf("centered connection name missing on a connection-scoped page: %s", inside)
	}

	for _, path := range []string{"/connections", "/connections/new"} {
		outside := mi.RenderToString(Page("Test", path, func(b *mi.Builder) mi.Node { return b.P("body") }))
		if strings.Contains(outside, `class="justify-self-center"><div`) {
			t.Fatalf("connection name present on %q, want it absent outside any specific connection: %s", path, outside)
		}
	}
}

func TestPage_DropdownMenuPresentOnlyInsideAConnection(t *testing.T) {
	inside := mi.RenderToString(Page("Test", "/connections/local/entities", func(b *mi.Builder) mi.Node { return b.P("body") }))
	if !strings.Contains(inside, "<details") {
		t.Fatalf("dropdown menu missing on a connection-scoped page: %s", inside)
	}

	for _, path := range []string{"/connections", "/connections/new"} {
		outside := mi.RenderToString(Page("Test", path, func(b *mi.Builder) mi.Node { return b.P("body") }))
		if strings.Contains(outside, "<details") {
			t.Fatalf("dropdown menu present on %q, want it absent outside any specific connection: %s", path, outside)
		}
	}
}

func TestPage_DropdownShowsDestinationsWithActiveHighlighting(t *testing.T) {
	html := mi.RenderToString(Page("Test", "/connections/local/blobs/somefolder", func(b *mi.Builder) mi.Node { return b.P("body") }))
	for _, want := range []string{`href="/connections/local/entities"`, `href="/connections/local/blobs"`, `href="/connections/local/query"`, `href="/connections/local/dxp"`, `href="/connections"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("dropdown missing %q: %s", want, html)
		}
	}
	// Blobs is active even on a subpath (browsing into a folder), not
	// just the exact /blobs URL.
	if !strings.Contains(html, `bg-indigo-50 dark:bg-indigo-900/30 text-indigo-700 dark:text-indigo-400 font-medium no-underline" href="/connections/local/blobs"`) {
		t.Fatalf("Blobs not marked active on a blobs subpath: %s", html)
	}
}

func TestPage_DropdownSwitcherListsOtherConnectionsNotSelf(t *testing.T) {
	store := connstore.NewFileBackend(t.TempDir())
	for _, name := range []string{"local", "staging", "prod"} {
		if _, err := store.Save(context.Background(), connstore.Connection{
			ConnectionMeta: connstore.ConnectionMeta{Name: name, BaseURL: "http://x", AuthMode: connstore.AuthNone},
		}); err != nil {
			t.Fatalf("seeding %q: %v", name, err)
		}
	}
	SetSidebarStore(store)
	defer SetSidebarStore(nil)

	html := mi.RenderToString(Page("Test", "/connections/local/entities", func(b *mi.Builder) mi.Node { return b.P("body") }))

	if strings.Contains(html, `href="/connections/local/entities">local`) {
		t.Fatalf("switcher lists the current connection itself: %s", html)
	}
	for _, want := range []string{"staging", "prod"} {
		if !strings.Contains(html, want) {
			t.Fatalf("switcher missing other connection %q: %s", want, html)
		}
	}
}

func TestPage_DropdownOmitsSwitcherWhenStoreUnset(t *testing.T) {
	SetSidebarStore(nil)
	html := mi.RenderToString(Page("Test", "/connections/local/entities", func(b *mi.Builder) mi.Node { return b.P("body") }))
	if strings.Contains(html, "Switch connection") {
		t.Fatalf("switcher section present with no store set: %s", html)
	}
	if !strings.Contains(html, "<details") {
		t.Fatalf("dropdown menu itself should still render without a store, just without the switcher: %s", html)
	}
}
