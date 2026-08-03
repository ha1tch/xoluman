// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	mi "github.com/ha1tch/minty"

	"github.com/ha1tch/xoluman/internal/connstore"
	"github.com/ha1tch/xoluman/internal/importer"
)

// maxImportUploadBytes bounds an import file's size outright — well
// beyond any reasonable row-based import, protecting against an
// accidental huge upload rather than a deliberate one; this is a
// single-operator local tool, not a public endpoint needing hardening
// against abuse.
const maxImportUploadBytes = 20 << 20 // 20MB

// ImportForm renders the upload page: a file input and a format choice.
func (h *EntitiesHandler) ImportForm(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entityType := r.PathValue("type")
	basePath := "/connections/" + name + "/entities/" + entityType

	body := func(b *mi.Builder) mi.Node {
		return b.Div(
			b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white mb-4"), "Import "+entityType),
			b.P(mi.Class("text-gray-600 dark:text-gray-400 text-sm mb-4"),
				"CSV (first row = field names) or a JSON array of objects. Each row is created independently — a failure on one row doesn't affect the others."),
			b.Form(mi.Attr("method", "post"), mi.Attr("action", basePath+"/import"), mi.Attr("enctype", "multipart/form-data"),
				b.Label(mi.For("format"), mi.Class(formLabelClass), "Format"),
				b.Select(mi.ID("format"), mi.Name("format"), mi.Class(formInputClass),
					b.Option(mi.Value("csv"), "CSV"),
					b.Option(mi.Value("json"), "JSON"),
				),
				b.Label(mi.For("file"), mi.Class(formLabelClass), "File"),
				b.Input(mi.Type("file"), mi.ID("file"), mi.Name("file"), mi.Required(), mi.Class(formInputClass)),
				b.Div(mi.Style("margin-top:1.5rem;"),
					b.Button(mi.Type("submit"), mi.Class(btnPrimary), "Preview import"),
				),
			),
		)
	}
	WriteHTML(w, Page("Import "+entityType, r.URL.Path, body))
}

// ImportPreview parses the uploaded file against the entity type's
// schema, stores the parsed rows as a session, and shows a preview —
// nothing is written to xolu yet.
func (h *EntitiesHandler) ImportPreview(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entityType := r.PathValue("type")
	basePath := "/connections/" + name + "/entities/" + entityType

	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	schema, err := c.GetEntitySchema(r.Context(), entityType)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxImportUploadBytes)
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		http.Error(w, "parsing upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "reading uploaded file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()

	var rows []importer.Row
	switch r.FormValue("format") {
	case "json":
		rows, err = importer.ParseJSON(file, schema.Fields)
	default:
		rows, err = importer.ParseCSV(file, schema.Fields)
	}
	if err != nil {
		http.Error(w, "parsing import file: "+err.Error(), http.StatusBadRequest)
		return
	}

	sessionID, err := h.sessions.Put(importer.Session{ConnName: name, EntityType: entityType, Rows: rows})
	if err != nil {
		http.Error(w, "starting import session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	body := importPreviewBody(entityType, basePath, sessionID, rows)
	WriteHTML(w, Page("Import "+entityType, r.URL.Path, body))
}

func importPreviewBody(entityType, basePath, sessionID string, rows []importer.Row) mi.H {
	return func(b *mi.Builder) mi.Node {
		okCount := 0
		for _, row := range rows {
			if row.OK() {
				okCount++
			}
		}
		errCount := len(rows) - okCount

		summary := fmt.Sprintf("%d row(s) parsed: %d ready to import, %d with errors (skipped).", len(rows), okCount, errCount)

		td := "px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm"
		rowNodes := make([]mi.Node, len(rows))
		for i, row := range rows {
			status := b.Span(mi.Class("text-green-600 dark:text-green-400"), "ok")
			if !row.OK() {
				var msgs []string
				for field, msg := range row.Errors {
					msgs = append(msgs, field+": "+msg)
				}
				status = b.Span(mi.Class("text-red-600 dark:text-red-400"), strings.Join(msgs, "; "))
			}
			rowNodes[i] = b.Tr(
				b.Td(mi.Class(td), strconv.Itoa(row.Index)),
				b.Td(mi.Class(td), status),
			)
		}
		table := Table([]string{"Row", "Status"}, rowNodes, "No rows found in the uploaded file.")(b)

		var confirmBtn interface{}
		if okCount > 0 {
			confirmBtn = b.Form(mi.Attr("method", "post"), mi.Attr("action", basePath+"/import/"+sessionID+"/confirm"), mi.Style("margin-top:1.5rem;"),
				b.Button(mi.Type("submit"), mi.Class(btnPrimary), fmt.Sprintf("Import %d row(s)", okCount)),
			)
		}

		return b.Div(
			b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white mb-2"), "Preview: import "+entityType),
			b.P(mi.Class("text-gray-600 dark:text-gray-400 text-sm mb-4"), summary),
			table,
			confirmBtn,
		)
	}
}

// ImportConfirm executes the previewed import: creates every row that
// parsed cleanly, independently — one row's failure does not affect the
// others, per the design recorded in internal/importer's package doc.
// The session is consumed exactly once by SessionStore.Take, so
// revisiting this URL after confirming does nothing.
func (h *EntitiesHandler) ImportConfirm(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entityType := r.PathValue("type")
	sessionID := r.PathValue("id")

	sess, ok := h.sessions.Take(sessionID)
	if !ok {
		http.Error(w, "import session not found or already used — re-upload the file to try again", http.StatusNotFound)
		return
	}

	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	type result struct {
		Index   int
		Success bool
		Message string
	}
	results := make([]result, 0, len(sess.Rows))
	succeeded := 0
	for _, row := range sess.Rows {
		if !row.OK() {
			continue // already known-invalid, never attempted — not a new failure to report
		}
		if _, err := c.Create(r.Context(), entityType, row.Values); err != nil {
			results = append(results, result{Index: row.Index, Success: false, Message: err.Error()})
			continue
		}
		results = append(results, result{Index: row.Index, Success: true})
		succeeded++
	}

	basePath := "/connections/" + name + "/entities/" + entityType
	body := func(b *mi.Builder) mi.Node {
		td := "px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm"
		rowNodes := make([]mi.Node, len(results))
		for i, res := range results {
			status := b.Span(mi.Class("text-green-600 dark:text-green-400"), "created")
			if !res.Success {
				status = b.Span(mi.Class("text-red-600 dark:text-red-400"), res.Message)
			}
			rowNodes[i] = b.Tr(b.Td(mi.Class(td), strconv.Itoa(res.Index)), b.Td(mi.Class(td), status))
		}
		table := Table([]string{"Row", "Result"}, rowNodes, "No rows were attempted.")(b)

		return b.Div(
			b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white mb-2"), "Import complete"),
			b.P(mi.Class("text-gray-600 dark:text-gray-400 text-sm mb-4"),
				fmt.Sprintf("%d of %d attempted row(s) created successfully.", succeeded, len(results))),
			table,
			b.A(mi.Href(basePath), mi.Class(btnSecondary), mi.Style("display:inline-flex; margin-top:1rem;"), "Back to "+entityType),
		)
	}
	WriteHTML(w, Page("Import complete", r.URL.Path, body))
}
