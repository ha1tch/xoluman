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
	"net/http"

	mi "github.com/ha1tch/minty"

	"github.com/ha1tch/xoluman/internal/modules"
)

// registry is package-level rather than threaded through every Page
// call — the standard shape for this kind of registration (database/sql
// drivers, image format decoders): whichever file owns a feature area
// registers itself in an init(), Page reads whatever's there. See
// connections.go's init() for the first (and currently only) entry.
var registry = modules.NewRegistry()

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
func PageWithHead(title, activePath string, extraHead []mi.Node, body mi.H) mi.H {
	return func(b *mi.Builder) mi.Node {
		head := []mi.Node{
			b.Meta(mi.Charset("UTF-8")),
			b.Meta(mi.Name("viewport"), mi.Content("width=device-width, initial-scale=1")),
			b.Title(title + " — xoluman"),
			b.Link(mi.Rel("stylesheet"), mi.Href("/static/css/tailwind.css")),
			b.Script(mi.Attr("src", "/static/vendor/htmx@1.9.10.min.js")),
			b.Script(mi.Attr("src", "/static/js/modal.js")),
		}
		head = append(head, extraHead...)
		headArgs := make([]interface{}, len(head))
		for i, n := range head {
			headArgs[i] = n
		}

		mods := registry.All()
		navLinks := make([]interface{}, 0, len(mods))
		for _, m := range mods {
			if m.Label == "" {
				continue // routes-only module (e.g. Entities, reached per-connection) — deliberately not a top-level nav entry
			}
			active := m.ActivePrefix != "" && hasPrefix(activePath, m.ActivePrefix)
			class := "text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white no-underline"
			if active {
				class = "text-gray-900 dark:text-white font-semibold no-underline"
			}
			navLinks = append(navLinks, b.A(mi.Href(m.URL), mi.Class(class), m.Label))
		}

		nav := b.Nav(mi.Class("flex items-center gap-6 px-5 py-3 border-b border-gray-200 dark:border-gray-700"),
			b.Div(mi.Class("font-semibold text-gray-900 dark:text-white"), "xoluman"),
			b.Div(append([]interface{}{mi.Class("flex gap-4")}, navLinks...)...),
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
