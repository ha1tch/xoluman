// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package ui provides xoluman's server-rendered pages, built with minty
// (github.com/ha1tch/minty) and htmx — no hand-written HTML, no React,
// per the project's standing convention. Every page goes through Page,
// which supplies the shared shell (nav, htmx/modal scripts, Tailwind
// stylesheet); individual pages provide only their body content.
//
// Styling follows Seam AMS's own system rather than a xoluman-specific
// one: Tailwind CSS, compiled at build time (never fetched at runtime —
// see docs/KNOWN_ISSUES.md's disconnected-operation decision) via
// `npm run css`, same config shape as Seam's own tailwind.config.js.
package ui

import (
	"context"
	"net/http"
	"net/url"

	mi "github.com/ha1tch/minty"

	"github.com/ha1tch/xoluman/internal/connstore"
	"github.com/ha1tch/xoluman/internal/modules"
)

// registry is package-level rather than threaded through every Page
// call — the standard shape for this kind of registration (database/sql
// drivers, image format decoders): whichever file owns a feature area
// registers itself in an init(), Page reads whatever's there. See
// connections.go's init() for the first (and currently only) entry.
var registry = modules.NewRegistry()

// sidebarStore is the same package-level-registration shape as
// registry above, for the same reason: the connection sidebar (see
// connectionSidebar) needs to list every connection for its "switch
// connection" section, and Page/PageWithHead are pure presentation
// functions with no store of their own to thread through every call
// site that renders a page. Set once at server startup via
// SetSidebarStore. nil until then — the sidebar simply omits the
// switcher section rather than panicking, which is also exactly what
// happens correctly in every existing test that never calls
// SetSidebarStore at all.
var sidebarStore connstore.Store

// SetSidebarStore registers the connection store the sidebar reads
// from — call once at server startup, alongside where the page's own
// route modules are registered.
func SetSidebarStore(store connstore.Store) {
	sidebarStore = store
}

// Page wraps body in xoluman's shared page shell: doctype, head
// (title, vendored htmx/modal scripts, the compiled Tailwind
// stylesheet), nav, and a main content area. activePath drives which
// nav entry is marked active — via each registered module's
// ActivePrefix, not a hardcoded list (T-08: three or more distinct
// feature areas, once the graph and FSM editors land, is exactly what
// a registry is for instead of hand-listing entries per page).
func Page(title, activePath string, body mi.H) mi.H {
	return PageWithHead(title, activePath, nil, body)
}

// PageWithHead is Page plus extra <head> content — for pages needing
// more than the baseline shell (an import map for a Lit shell, an
// extra vendored stylesheet like Tabulator's). Kept as a separate
// function rather than changing Page's signature, which would touch
// every existing call site across the codebase for something only a
// handful of richer pages (the grid editor, and later the FSM/graph
// editors) actually need.
// connectionNameFromPath extracts the connection name from a
// /connections/{name}/... path — but not from /connections or
// /connections/new themselves, which aren't "inside" any specific
// connection. Used to decide whether to show the connection sidebar,
// entirely from the URL already being passed to every page — no
// caller needs to be touched to thread a connection name through
// separately.
func connectionNameFromPath(path string) (string, bool) {
	const prefix = "/connections/"
	if !hasPrefix(path, prefix) {
		return "", false
	}
	rest := path[len(prefix):]
	if rest == "" || rest == "new" {
		return "", false
	}
	if i := indexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	if rest == "" || rest == "new" {
		return "", false
	}
	name, err := url.QueryUnescape(rest)
	if err != nil {
		return rest, true // fall back to the raw segment rather than losing the sidebar over a decode edge case
	}
	return name, true
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// connectionSidebar is a real answer to a direct report: returning to
// the connections list to switch context, or to reach Blobs/Query
// after already being in Entities, made no sense once already working
// inside a connection. Shown on every /connections/{name}/... page —
// parsed from activePath itself (see connectionNameFromPath), not
// something every page handler needs to pass in separately.
//
// Deliberately scoped for now to a connection switcher and the three
// per-connection destinations (Entities/Blobs/Query) — not per-entity-
// type sub-links (which would mean an extra ListEntities call on every
// single page load just to populate a sidebar) and not real multi-tab
// support (a genuinely bigger, separate piece of work — see
// docs/proposals/dbeaver-layout.md's own Phase 1/Phase 2 split).
// connectionMenu builds the dropdown panel's content for the current
// connection — destinations (Entities/Blobs/Query/DXP) with
// active-state highlighting, then "All connections", then a "switch
// connection" list of every other saved connection. Lives inside a
// native <details>/<summary> dropdown in the nav bar (see
// PageWithHead) rather than the earlier persistent left sidebar —
// reported directly as looking bad in that position; <details> is
// deliberately native, zero-JS, browser-handled open/close, not a
// second custom JS component to get subtly wrong the way modal.js's
// own htmx listener just was (a real bug, caught and fixed the same
// session, from adding JS where none was strictly needed).
func connectionMenu(b *mi.Builder, connName, activePath string) mi.Node {
	destinations := []struct {
		label, suffix string
	}{
		{"Entities", "/entities"},
		{"Blobs", "/blobs"},
		{"Query", "/query"},
		{"DXP", "/dxp"},
		{"FSM", "/fsm"},
		{"Graph", "/graph"},
	}

	itemClass := "block px-3 py-1.5 text-sm text-gray-700 dark:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-700 no-underline"
	activeItemClass := "block px-3 py-1.5 text-sm bg-indigo-50 dark:bg-indigo-900/30 text-indigo-700 dark:text-indigo-400 font-medium no-underline"

	links := make([]interface{}, 0, len(destinations))
	base := "/connections/" + url.PathEscape(connName)
	for _, d := range destinations {
		href := base + d.suffix
		active := activePath == href || (len(activePath) > len(href) && activePath[:len(href)+1] == href+"/")
		class := itemClass
		if active {
			class = activeItemClass
		}
		links = append(links, b.A(mi.Href(href), mi.Class(class), d.label))
	}

	items := append([]interface{}{}, links...)
	items = append(items,
		b.Div(mi.Class("my-1 border-t border-gray-200 dark:border-gray-700")),
		b.A(mi.Href("/connections"), mi.Class(itemClass), "All connections"),
	)

	if sidebarStore != nil {
		if all, err := sidebarStore.List(context.Background()); err == nil {
			otherConns := make([]interface{}, 0, len(all))
			for _, c := range all {
				if c.Name == connName {
					continue
				}
				href := "/connections/" + url.PathEscape(c.Name) + "/entities"
				otherConns = append(otherConns, b.A(mi.Href(href), mi.Class(itemClass+" truncate"), c.Name))
			}
			if len(otherConns) > 0 {
				items = append(items,
					b.Div(mi.Class("my-1 border-t border-gray-200 dark:border-gray-700")),
					b.Div(mi.Class("px-3 py-1 text-xs font-semibold text-gray-400 dark:text-gray-500 uppercase tracking-wider"), "Switch connection"),
				)
				items = append(items, otherConns...)
			}
		}
	}

	return b.Div(append([]interface{}{mi.Class("py-1")}, items...)...)
}

func PageWithHead(title, activePath string, extraHead []mi.Node, body mi.H) mi.H {
	return func(b *mi.Builder) mi.Node {
		// minty ships a complete, tested dark-mode mechanism
		// (darkmode.go, documented in its own DARKMODE.md) —
		// detection, localStorage persistence, system-preference
		// fallback, and a toggle button with an auto-updating icon.
		// This replaced a hand-rolled theme.js that reimplemented
		// the same thing from scratch, less completely (plain text
		// glyphs instead of real icons, no icon auto-update
		// handoff) — using what the library already provides
		// properly, not a from-scratch version of it.
		darkMode := mi.DarkModeTailwind(mi.DarkModeSVGIcons())

		head := []mi.Node{
			b.Meta(mi.Charset("UTF-8")),
			b.Meta(mi.Name("viewport"), mi.Content("width=device-width, initial-scale=1")),
			b.Title(title + " — xoluman"),
			// darkMode.Script first and deliberately not deferred —
			// setting the dark/light class has to happen before
			// first paint, not after the stylesheet or any other
			// script has had a chance to render anything.
			darkMode.Script(b),
			b.Link(mi.Rel("stylesheet"), mi.Href("/static/css/tailwind.css")),
			b.Script(mi.Attr("src", "/static/vendor/htmx@1.9.10.min.js")),
			b.Script(mi.Attr("src", "/static/js/modal.js")),
		}
		head = append(head, extraHead...)
		headArgs := make([]interface{}, len(head))
		for i, n := range head {
			headArgs[i] = n
		}

		// The brand mark doubles as the home link — xoluman has
		// exactly one meaningful top-level destination (the connection
		// list), and every single page in the app lives somewhere
		// under /connections/..., which made a separate "Connections"
		// nav item permanently render in its own active state: no
		// real feedback, and by report ("never highlighted, not a
		// tab, not a recognisable UI element") it just looked broken.
		// One honest, always-clickable brand/home link plus the theme
		// toggle replaces it — nothing pretending to be a multi-item
		// nav that isn't one.
		themeToggle := darkMode.Toggle(b,
			mi.Class("p-1.5 rounded-lg cursor-pointer border-0 bg-transparent text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 hover:text-gray-900 dark:hover:text-white"),
		)

		connName, insideConnection := connectionNameFromPath(activePath)

		// Center column: the current connection's name when inside
		// one — reported directly as wanting real prominence rather
		// than being buried in a sidebar, so it gets the visually
		// central position in the bar itself.
		var centerContent interface{} = ""
		if insideConnection {
			centerContent = b.Div(mi.Class("font-semibold text-gray-900 dark:text-white truncate max-w-xs"), connName)
		}

		// Right column: theme toggle plus, when inside a connection, a
		// native <details>/<summary> dropdown replacing the earlier
		// persistent left sidebar (reported directly as looking bad
		// where it was) — Entities/Blobs/Query/DXP, All connections,
		// and a connection switcher, all in one place instead of
		// permanently occupying page width.
		rightItems := []interface{}{themeToggle}
		if insideConnection {
			menuButtonClass := "flex items-center gap-1 px-2.5 py-1.5 rounded-lg text-sm text-gray-700 dark:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-700 cursor-pointer select-none list-none"
			panelClass := "absolute right-0 mt-1 w-52 rounded-lg shadow-lg ring-1 ring-black/5 dark:ring-white/10 bg-white dark:bg-gray-800 z-10"
			rightItems = append(rightItems, b.Details(mi.Class("relative"),
				b.Summary(mi.Class(menuButtonClass), "Menu ▾"),
				b.Div(mi.Class(panelClass), connectionMenu(b, connName, activePath)),
			))
		}

		// A three-column grid, not flex, specifically because a flex
		// row can't center the middle item independently of how wide
		// the left/right content happens to be — grid-template-
		// columns: 1fr auto 1fr gives the center column a stable,
		// truly-centered position regardless of the brand link's or
		// the right-side controls' own width.
		nav := b.Nav(mi.Class("grid grid-cols-[1fr_auto_1fr] items-center gap-4 px-5 py-3 border-b border-gray-200 dark:border-gray-700"),
			b.Div(mi.Class("justify-self-start"),
				b.A(mi.Href("/connections"), mi.Class("font-semibold text-gray-900 dark:text-white no-underline hover:text-indigo-600 dark:hover:text-indigo-400"), "xoluman"),
			),
			b.Div(mi.Class("justify-self-center"), centerContent),
			b.Div(append([]interface{}{mi.Class("justify-self-end flex items-center gap-2")}, rightItems...)...),
		)

		main := b.Main(mi.Class("max-w-5xl mx-auto p-5"), body(b))

		return mi.NewFragment(
			mi.Raw("<!DOCTYPE html>"),
			b.Html(mi.Lang("en"),
				b.Head(headArgs...),
				b.Body(mi.Class("bg-white dark:bg-gray-900 text-gray-900 dark:text-gray-100 font-sans min-h-screen m-0"),
					nav, main),
			),
		)
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// WriteHTML renders template and writes it to w as text/html with a 200
// status. Handlers call this once they have their page assembled.
func WriteHTML(w http.ResponseWriter, template mi.H) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = mi.Render(template, w)
}
