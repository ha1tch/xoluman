// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"net/http"

	mi "github.com/ha1tch/minty"
)

// btnPrimary, btnSecondary, btnDanger match Seam's own button classes
// (internal/ui/listing.go / components.go) so xoluman's visual language
// is Seam's, not a new one.
const (
	btnPrimary   = "inline-flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 text-sm font-medium cursor-pointer border-0"
	btnSecondary = "inline-flex items-center gap-2 px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-300 dark:hover:bg-gray-600 text-sm font-medium cursor-pointer border-0"
	btnDanger    = "inline-flex items-center gap-2 px-4 py-2 bg-red-50 text-red-700 rounded-lg hover:bg-red-100 dark:bg-red-900/30 dark:text-red-400 text-sm font-medium cursor-pointer border-0"
)

// ListPageConfig configures a listing page's header and its optional
// modal-triggered create action.
type ListPageConfig struct {
	Title       string
	CreateLabel string // empty → no create button
	CreateURL   string // GET endpoint returning the create-form fragment, loaded into the modal
}

// ListPage renders the standard listing page shell: a header with an
// optional create button that opens the modal, followed by content
// (typically a Table). This is the one shape every table-style listing
// page in xoluman uses — adding a new listing means calling this, not
// hand-building a header and table each time.
func ListPage(cfg ListPageConfig, content mi.H) mi.H {
	return func(b *mi.Builder) mi.Node {
		var createBtn interface{}
		if cfg.CreateURL != "" {
			createBtn = ModalTriggerButton("+ "+cfg.CreateLabel, cfg.CreateLabel, cfg.CreateURL, btnPrimary)(b)
		}
		header := b.Div(mi.Class("flex items-center justify-between mb-4"),
			b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white m-0"), cfg.Title),
			createBtn,
		)
		return b.Div(header, content(b))
	}
}

// Table renders columns as a header row and rows as the body, or
// emptyMessage in an empty-state block when rows is empty. Rows are
// typically built with TableRow (or hand-built b.Tr(...) calls) by the
// caller — Table itself only assembles the shell.
// Table wraps rows in the visual conventions minty's own Tailwind
// theme establishes for a data table (themes/tailwind/tailwind.go's
// Table()) — a real elevated card (shadow, subtle ring, rounded
// corners), a distinct header background with uppercase tracked text,
// and striped rows — none of which xoluman's own implementation had
// adopted before this, despite depending on minty throughout. A real
// gap, not a style preference: reported directly as looking "too
// default, no signs of styling attempts."
//
// Striping is applied via a Tailwind arbitrary-variant selector on
// <tbody> targeting even child rows, rather than requiring every
// caller to stripe its own <tr> elements — every one of the many
// call sites across this codebase gets it automatically, not just
// whichever ones remember to ask.
func Table(columns []string, rows []mi.Node, emptyMessage string) mi.H {
	return func(b *mi.Builder) mi.Node {
		if len(rows) == 0 {
			return b.Div(mi.Class("text-center py-12 text-gray-500 dark:text-gray-400 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700"), emptyMessage)
		}

		headerCells := make([]interface{}, len(columns))
		for i, c := range columns {
			headerCells[i] = b.Th(mi.Class("px-3 py-3 text-left text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider"), c)
		}

		rowArgs := make([]interface{}, len(rows))
		for i, r := range rows {
			rowArgs[i] = r
		}

		return b.Div(mi.Class("overflow-hidden shadow-sm ring-1 ring-black/5 dark:ring-white/10 rounded-lg"),
			b.Table(mi.Class("w-full border-collapse"),
				b.Thead(mi.Class("bg-gray-50 dark:bg-gray-800/60 border-b border-gray-200 dark:border-gray-700"), b.Tr(headerCells...)),
				b.Tbody(append([]interface{}{mi.Class("[&>tr:nth-child(even)]:bg-gray-50 dark:[&>tr:nth-child(even)]:bg-gray-800/40")}, rowArgs...)...),
			),
		)
	}
}

// ModalTriggerButton renders a <button> that opens the shared modal
// (XModal, web/static/js/modal.js) with modalTitle, loading url's
// response into the modal body via htmx. buttonLabel is the button's own
// text, which may differ from modalTitle (e.g. "+ New connection" as the
// button, "New connection" as the modal title).
func ModalTriggerButton(buttonLabel, modalTitle, url, class string) mi.H {
	return func(b *mi.Builder) mi.Node {
		return b.Button(
			mi.Type("button"), mi.Class(class),
			mi.HxGet(url), mi.HxTarget("#modal-body"), mi.HxSwap("innerHTML"),
			mi.Attr("onclick", "XModal.open('"+modalTitle+"')"),
			buttonLabel,
		)
	}
}

// lightningIconSVG is a small bolt icon, matching the outline style
// (stroke="currentColor", no fill) minty's own DarkModeSVGIcons use —
// a real icon, not a text glyph, per the same lesson the theme toggle
// itself needed applied here directly.
const lightningIconSVG = `<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>`

// RefJumpButton opens a ref field's linked entity directly in a modal
// — a real, requested navigation shortcut: previously the only way to
// look at what a reference actually points to was the ref link itself,
// a full page navigation away from wherever it was clicked. The
// button's own hx-get title param is a placeholder ("Loading…") —
// modal.js's own htmx:afterSwap listener promotes the loaded form's
// real <h1> to the modal's title bar once it arrives, the same
// mechanism every other modal-opened form already uses, so this
// doesn't need to know the specific title in advance.
func RefJumpButton(editURL string) mi.H {
	return func(b *mi.Builder) mi.Node {
		return b.Button(
			mi.Type("button"),
			mi.Class("inline-flex items-center ml-1 text-gray-400 dark:text-gray-500 hover:text-indigo-600 dark:hover:text-indigo-400 cursor-pointer border-0 bg-transparent p-0 align-middle"),
			mi.Attr("title", "Open in a modal"),
			mi.HxGet(editURL), mi.HxTarget("#modal-body"), mi.HxSwap("innerHTML"),
			mi.Attr("onclick", "XModal.open('Loading…')"),
			mi.Raw(lightningIconSVG),
		)
	}
}

// DeleteConfirmBody renders a modal-body confirmation: itemLabel names
// what's being deleted, deleteURL is the plain form POST target (a real
// navigation/redirect on submit, not htmx — the same convention the
// create form already uses). Cancel closes the modal without submitting.
func DeleteConfirmBody(itemLabel, deleteURL string) mi.H {
	return func(b *mi.Builder) mi.Node {
		return b.Div(
			b.P(mi.Class("text-gray-700 dark:text-gray-300"), "Delete "+itemLabel+"? This cannot be undone."),
			b.Div(mi.Class("flex gap-2 mt-4"),
				b.Form(mi.Attr("method", "post"), mi.Attr("action", deleteURL),
					b.Button(mi.Type("submit"), mi.Class(btnDanger), "Delete"),
				),
				b.Button(mi.Type("button"), mi.Class(btnSecondary), mi.Attr("onclick", "XModal.close()"), "Cancel"),
			),
		)
	}
}

// WriteModalAware writes fragment (bare, no page shell) when the request
// came from htmx — i.e. it's being loaded into the shared modal — and
// falls back to the full page shell (fullPageBody wrapped in Page) for a
// direct navigation, e.g. someone bookmarked or typed the URL. Every
// modal-loaded GET handler in xoluman goes through this rather than
// assuming htmx, so a direct hit doesn't render a bare, shell-less
// fragment as if it were a whole page.
func WriteModalAware(w http.ResponseWriter, r *http.Request, title string, fragment mi.H) {
	if mi.IsHTMX(r) {
		WriteFragment(w, fragment)
		return
	}
	WriteHTML(w, Page(title, r.URL.Path, fragment))
}
