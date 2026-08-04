// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package importer parses uploaded CSV/JSON files into rows ready to
// create as entities, against a target entity type's schema.
//
// Design, recorded here since these are real decisions with real
// trade-offs, not defaults that happened by accident:
//
//   - No native xolu import endpoint exists (confirmed in T-05's own
//     tracking entry) — import is entity-by-entity Create calls, not a
//     server-side bulk operation. Rows are therefore not atomic as a
//     whole: each row succeeds or fails independently, and Execute
//     reports per-row results rather than an all-or-nothing outcome.
//     This matches how most file-import tools of this shape behave and
//     is far simpler than trying to fake transactional semantics xolu
//     doesn't provide.
//   - CSV cells are always strings; a CSV row is therefore the exact
//     same shape as a submitted HTML form once you're past reading the
//     file, so CSV parsing reuses formengine.ParseFormValues rather
//     than re-implementing type coercion a second time.
//   - JSON values already carry their own types (numbers as float64,
//     booleans as bool, per encoding/json's decode-into-any shapes), so
//     JSON rows only get a lighter check — required fields present and
//     non-null. A field whose JSON type doesn't match the schema is
//     deliberately not caught here; it surfaces as a real per-row error
//     from xolu's own validator at Execute time. Building a full
//     client-side JSON-Schema type checker for a preview step is more
//     machinery than this needs for a first version.
//   - Ref fields expect a raw numeric target ID in the input file. No
//     lookup-by-name convenience exists yet — a real feature, not
//     built here.
package importer

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/formengine"
)

// Row is one parsed record. Values holds successfully-coerced fields;
// Errors holds per-field problems. A row with any Errors is skipped
// during Execute, not partially imported.
type Row struct {
	Index  int // 1-based, matching the line/element position in the source file — for human-readable error reporting
	Values formengine.Values
	Errors formengine.Errors
}

// OK reports whether the row parsed cleanly and is eligible to import.
func (r Row) OK() bool { return len(r.Errors) == 0 }

// ParseCSV parses r as CSV with a header row. Header cells are matched
// against fields by name; columns that don't match any known field are
// silently ignored rather than erroring — an export from elsewhere with
// extra columns should still import the columns it recognises.
//
// refTargets maps a ref field's name to its target entity type (see
// internal/ui's refTargetsByField) — needed because xolu requires ref
// values in a structured write shape, not a bare ID (confirmed
// directly against a real server: XOLU-VL001, "expected
// {type,entity,id}, got float64"); this package delegates that
// construction to formengine.ParseFormValues, which needs the same
// info. Pass nil when unavailable (schema-less/inferred fields) — a
// ref column then imports as a plain number and fails xolu's own
// validation on write with a clear error, not a silent guess.
//
// Boolean fields are deliberately not delegated to
// formengine.ParseFormValues along with everything else: that function's
// boolean handling is checkbox-shaped (a present form key means
// checked, regardless of its value — correct for HTML, where an
// unchecked box is omitted from the submission entirely). A CSV column
// is not a checkbox: it can be present with an empty or literal
// "false" cell, and treating presence alone as true would silently
// flip every boolean column true the moment it's included in the file
// at all. Booleans get their own value-based parse here instead.
func ParseCSV(r io.Reader, fields []client.FieldDef, refTargets map[string]string) ([]Row, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // tolerate ragged rows rather than aborting the whole file on one short line

	header, err := cr.Read()
	if err == io.EOF {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading CSV header: %w", err)
	}

	var rows []Row
	index := 1
	for {
		record, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return rows, fmt.Errorf("reading CSV row %d: %w", index, err)
		}

		form := url.Values{}
		cellByColumn := make(map[string]string, len(header))
		for i, col := range header {
			if i >= len(record) {
				break // ragged row shorter than the header — remaining columns simply absent, same as any other absent field
			}
			cellByColumn[col] = record[i]
			form.Set(col, record[i])
		}

		values, errs := formengine.ParseFormValues(fields, form, refTargets)
		resolveCSVBooleans(fields, cellByColumn, values, errs)

		rows = append(rows, Row{Index: index, Values: values, Errors: errs})
		index++
	}
	return rows, nil
}

// resolveCSVBooleans overrides whatever formengine.ParseFormValues
// guessed for boolean fields (presence-based, wrong for CSV) with a
// value-based interpretation of the actual cell content.
func resolveCSVBooleans(fields []client.FieldDef, cellByColumn map[string]string, values formengine.Values, errs formengine.Errors) {
	for _, f := range fields {
		if f.Type != "boolean" {
			continue
		}
		cell, present := cellByColumn[f.Name]
		if !present {
			delete(values, f.Name) // column absent from this file entirely — omit, don't default to false
			continue
		}
		b, err := parseCSVBool(cell)
		if err != nil {
			errs[f.Name] = err.Error()
			delete(values, f.Name)
			continue
		}
		values[f.Name] = b
	}
}

// parseCSVBool interprets a CSV cell as a boolean by its value, not its
// mere presence. Empty cells resolve to false rather than erroring —
// an explicitly-blank boolean cell reads as "not set" more naturally
// than as a validation failure.
func parseCSVBool(cell string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(cell)) {
	case "", "false", "0", "no", "n", "off":
		return false, nil
	case "true", "1", "yes", "y", "on":
		return true, nil
	default:
		return false, fmt.Errorf("must be true/false (got %q)", cell)
	}
}

// ParseJSON parses r as a JSON array of objects, one per entity to
// import.
//
// refTargets maps a ref field's name to its target entity type, same
// as ParseCSV — needed because xolu requires ref values in a
// structured write shape ({"type":"REF","entity":"<target>","id":N}),
// not a bare ID (confirmed directly against a real server:
// XOLU-VL001, "expected {type,entity,id}, got float64"). A bare number
// is the natural way someone would hand-author a ref value in a JSON
// import file, so it's normalized automatically when the target is
// known; an already-structured value passes through unchanged. Pass
// nil refTargets when unavailable — a ref field then imports as
// whatever was literally written and fails xolu's own validation on
// write with a clear error if that happens to be a bare number, not a
// silent guess at which entity type it points at.
func ParseJSON(r io.Reader, fields []client.FieldDef, refTargets map[string]string) ([]Row, error) {
	var records []map[string]any
	if err := json.NewDecoder(r).Decode(&records); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	rows := make([]Row, len(records))
	for i, rec := range records {
		errs := make(formengine.Errors)
		for _, f := range fields {
			v, present := rec[f.Name]
			if f.Required && (!present || v == nil) {
				errs[f.Name] = "This field is required."
				continue
			}
			if !present || v == nil {
				continue
			}
			if (f.Format == "ref" || f.Type == "ref") && refTargets[f.Name] != "" {
				rec[f.Name] = normalizeRefValue(v, refTargets[f.Name])
			}
		}
		rows[i] = Row{Index: i + 1, Values: formengine.Values(rec), Errors: errs}
	}
	return rows, nil
}

// normalizeRefValue ensures a ref field's JSON-import value is in
// xolu's required structured write shape, wrapping a bare number (the
// natural way someone would hand-author a ref in an import file) if
// that's what was given. An already-structured value (someone who
// already knew the write shape) passes through unchanged, not
// double-wrapped or altered.
func normalizeRefValue(v any, target string) any {
	if n, ok := v.(float64); ok {
		return map[string]any{"type": "REF", "entity": target, "id": int64(n)}
	}
	return v // already structured, or a shape this can't help with — xolu's own validation is the right place for that to fail, not a guess here
}
