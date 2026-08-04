// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	mi "github.com/ha1tch/minty"
	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/connstore"
)

// PromotePreview renders a schema suggestion for entityType — no side
// effects, purely a look, per Client.GetSchemaSuggestion's own contract
// (confirmed: samples up to 500 rows, infers, never applies anything).
func (h *EntitiesHandler) PromotePreview(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entityType := r.PathValue("type")

	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	suggestion, err := c.GetSchemaSuggestion(r.Context(), entityType)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	schemaJSON, err := json.MarshalIndent(suggestion.SuggestedSchema, "", "  ")
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	basePath := entitiesBasePath(name, entityType)
	body := promotePreviewBody(entityType, basePath, suggestion, string(schemaJSON), "")
	WriteHTML(w, Page("Promote "+entityType, r.URL.Path, body))
}

func promotePreviewBody(entityType, basePath string, suggestion *xclient.SchemaSuggestion, schemaJSON, generalErr string) mi.H {
	return func(b *mi.Builder) mi.Node {
		var errNode interface{}
		if generalErr != "" {
			errNode = b.Div(mi.Class("text-red-600 dark:text-red-400 text-sm mb-3"), generalErr)
		}

		rows := make([]mi.Node, len(suggestion.FieldAnalysis))
		for i, f := range suggestion.FieldAnalysis {
			note := f.Note
			if len(f.SuggestedEnum) > 0 {
				if note != "" {
					note += "; "
				}
				note += "possible enum: " + strings.Join(f.SuggestedEnum, ", ")
			}
			rows[i] = b.Tr(
				b.Td(mi.Class("px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm"), f.Field),
				b.Td(mi.Class("px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm"), f.InferredType),
				b.Td(mi.Class("px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm"), fmt.Sprintf("%.0f%%", f.Coverage*100)),
				b.Td(mi.Class("px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm"), f.Confidence),
				b.Td(mi.Class("px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm text-gray-500 dark:text-gray-400"), note),
			)
		}
		analysisTable := Table([]string{"Field", "Type", "Coverage", "Confidence", "Notes"}, rows, "")(b)

		return b.Div(
			b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white mb-2"), "Promote "+entityType+" to a schema"),
			b.P(mi.Class("text-gray-600 dark:text-gray-400 text-sm mb-4"),
				fmt.Sprintf("Sampled %d of %d row(s). Nothing is applied until you submit below.", suggestion.SampledRows, suggestion.TotalRows)),
			errNode,
			analysisTable,
			b.Form(mi.Attr("method", "post"), mi.Attr("action", basePath+"/promote"), mi.Style("margin-top:1.5rem;"),
				b.Label(mi.For("schema"), mi.Class(formLabelClass), "Schema (edit before promoting if you want to)"),
				b.Textarea(mi.ID("schema"), mi.Name("schema"), mi.Class(formInputClass+" font-mono"), mi.Attr("rows", "14"), schemaJSON),
				b.Div(mi.Class("mt-4 flex gap-3 items-start"),
					b.Button(mi.Type("submit"), mi.Name("mode"), mi.Value("strict"), mi.Class(btnPrimary),
						"Promote (strict — validates every row first, recommended)"),
					b.Button(mi.Type("submit"), mi.Name("mode"), mi.Value("flex"), mi.Class(btnSecondary),
						"Promote (flex — faster, may hide existing rows)"),
				),
				b.P(mi.Class("text-xs text-gray-500 dark:text-gray-400 mt-2"),
					"Flex does not migrate rows that already exist into the new adapted storage — "+
						"they stay reachable by ID but disappear from lists and counts until rewritten. "+
						"Strict checks every existing row against the schema first and only promotes if all of them pass."),
			),
		)
	}
}

// Promote applies a schema-promotion decision — flex or strict,
// selected by which submit button was pressed.
func (h *EntitiesHandler) Promote(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entityType := r.PathValue("type")

	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "parsing form: "+err.Error(), http.StatusBadRequest)
		return
	}

	var schema map[string]interface{}
	schemaText := r.PostForm.Get("schema")
	if err := json.Unmarshal([]byte(schemaText), &schema); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		// Re-fetch the suggestion for a clean redisplay rather than
		// trying to reconstruct FieldAnalysis from nothing — a
		// malformed-JSON edit is expected to be rare enough that one
		// extra round trip here is a reasonable trade for not
		// duplicating GetSchemaSuggestion's shape by hand.
		suggestion, sErr := c.GetSchemaSuggestion(r.Context(), entityType)
		if sErr != nil {
			writeUpstreamError(w, name, r.URL.Path, sErr)
			return
		}
		basePath := entitiesBasePath(name, entityType)
		body := promotePreviewBody(entityType, basePath, suggestion, schemaText, "Invalid JSON: "+err.Error())
		WriteHTML(w, Page("Promote "+entityType, r.URL.Path, body))
		return
	}

	basePath := entitiesBasePath(name, entityType)

	switch r.PostForm.Get("mode") {
	case "flex":
		result, err := c.PromoteFlex(r.Context(), entityType, schema)
		if err != nil {
			writeUpstreamError(w, name, r.URL.Path, err)
			return
		}
		WriteHTML(w, Page("Promote "+entityType, r.URL.Path, promoteFlexResultBody(entityType, basePath, result)))
	case "strict":
		job, err := c.PromoteStrict(r.Context(), entityType, schema)
		if err != nil {
			writeUpstreamError(w, name, r.URL.Path, err)
			return
		}
		WriteHTML(w, Page("Promote "+entityType, r.URL.Path, promoteStrictResultBody(entityType, basePath, job)))
	default:
		http.Error(w, "unknown promotion mode", http.StatusBadRequest)
	}
}

func promoteFlexResultBody(entityType, basePath string, result *xclient.PromoteFlexResult) mi.H {
	return func(b *mi.Builder) mi.Node {
		var warning interface{}
		if result.Warning != "" {
			warning = b.Div(mi.Class("text-yellow-600 dark:text-yellow-400 text-sm mt-3"), result.Warning)
		}
		return b.Div(
			b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white mb-2"), entityType+" promoted"),
			b.P(mi.Class("text-gray-600 dark:text-gray-400 text-sm"), result.Message),
			warning,
			b.A(mi.Href(basePath), mi.Class(btnSecondary), mi.Style("display:inline-flex; margin-top:1rem;"), "Back to "+entityType),
		)
	}
}

func promoteStrictResultBody(entityType, basePath string, job *xclient.PromoteJob) mi.H {
	return func(b *mi.Builder) mi.Node {
		if job.Status == xclient.PromoteJobRejected {
			rows := make([]mi.Node, len(job.Failures))
			for i, f := range job.Failures {
				rows[i] = b.Tr(
					b.Td(mi.Class("px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm"), strconv.Itoa(f.ID)),
					b.Td(mi.Class("px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm text-red-600 dark:text-red-400"), strings.Join(f.Errors, "; ")),
				)
			}
			table := Table([]string{"Row ID", "Errors"}, rows, "")(b)
			return b.Div(
				b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white mb-2"), "Promotion declined"),
				b.P(mi.Class("text-gray-600 dark:text-gray-400 text-sm mb-4"),
					"Not every existing row validated against this schema, so nothing was changed — "+entityType+" is still exactly as schemaless as it was. Fix the data or loosen the schema and try again."),
				table,
				b.A(mi.Href(basePath+"/promote"), mi.Class(btnSecondary), mi.Style("display:inline-flex; margin-top:1rem;"), "Back to preview"),
			)
		}

		migrated := 0
		if job.Result != nil {
			migrated = job.Result.MigratedRows
		}
		return b.Div(
			b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white mb-2"), entityType+" promoted"),
			b.P(mi.Class("text-gray-600 dark:text-gray-400 text-sm"), fmt.Sprintf("%d row(s) validated and migrated.", migrated)),
			b.A(mi.Href(basePath), mi.Class(btnSecondary), mi.Style("display:inline-flex; margin-top:1rem;"), "Back to "+entityType),
		)
	}
}
