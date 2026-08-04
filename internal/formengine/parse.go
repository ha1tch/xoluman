// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package formengine

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/ha1tch/xolu/pkg/client"
)

// ParseFormValues converts a submitted form back into a map[string]any
// matching each field's JSON Schema type — the inverse of RenderFields.
// Malformed values (non-numeric text in a number field, invalid JSON in
// an object/array textarea, an empty required field) are collected into
// Errors rather than aborting the whole parse, so the caller can
// redisplay the form with per-field messages instead of losing
// everything the person already typed.
//
// Fields absent from form entirely (not just empty-string) are treated
// as not submitted and omitted from the result, except booleans: an
// absent boolean field means an unchecked checkbox, which browsers omit
// from form data entirely rather than sending false — this is handled
// explicitly, not left as an accidental omission.
//
// refTargets maps a ref field's name to its target entity type (see
// internal/ui's refTargetsByField, built from EntitySchema.Refs) — a
// ref field can only be correctly constructed with a known target,
// since xolu requires the structured write shape
// {"type":"REF","entity":"<target>","id":<int>}, confirmed directly
// against a real server: a bare integer is rejected with XOLU-VL001
// ("expected {type,entity,id}, got float64"). Pass nil when target
// info isn't available (e.g. a schema-less/inferred field list, which
// has no way to know a field is even a reference, let alone its
// target) — a ref field with no entry in refTargets is treated as an
// ordinary numeric field, which will fail xolu's own validation on
// write with a clear error rather than xoluman silently sending
// something wrong without saying so.
func ParseFormValues(fields []client.FieldDef, form url.Values, refTargets map[string]string) (Values, Errors) {
	values := make(Values, len(fields))
	errs := make(Errors)

	for _, f := range fields {
		if f.Type == "boolean" {
			_, present := form[f.Name]
			values[f.Name] = present
			continue
		}

		if _, present := form[f.Name]; !present {
			continue
		}
		raw := form.Get(f.Name)

		if f.Required && strings.TrimSpace(raw) == "" {
			errs[f.Name] = "This field is required."
			continue
		}
		if raw == "" {
			// Optional and empty: omit rather than storing an empty
			// string for a numeric/object field, which would fail
			// xolu's own type validation on write.
			continue
		}

		switch {
		case (f.Format == "ref" || f.Type == "ref") && refTargets[f.Name] != "":
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				errs[f.Name] = "Must be a valid ID."
				continue
			}
			values[f.Name] = map[string]any{"type": "REF", "entity": refTargets[f.Name], "id": id}
		case f.Format == "ref" || f.Type == "ref":
			// No known target (schema-less/inferred, or a polymorphic
			// ref with no single target type) — falls back to a bare
			// numeric value. xolu will reject this on write with a
			// clear validation error; that's the correct outcome here,
			// not a silent guess at which entity type this points at.
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				errs[f.Name] = "Must be a valid ID."
				continue
			}
			values[f.Name] = v
		case f.Type == "integer":
			v, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				errs[f.Name] = "Must be a whole number."
				continue
			}
			values[f.Name] = float64(v) // Values holds decoded-JSON shapes; integers are float64 same as everywhere else in this package
		case f.Type == "number":
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				errs[f.Name] = "Must be a number."
				continue
			}
			values[f.Name] = v
		case f.Format == "decimal":
			// Deliberately not parsed to a numeric type at all — stays
			// the exact string submitted, same reasoning as
			// RenderFields never using <input type="number"> for it:
			// avoiding float64 precision loss end to end, render to
			// submit.
			values[f.Name] = raw
		case f.Type == "object" || f.Type == "array":
			var v any
			if err := json.Unmarshal([]byte(raw), &v); err != nil {
				errs[f.Name] = "Must be valid JSON."
				continue
			}
			values[f.Name] = v
		default: // "string" and anything else unrecognised
			values[f.Name] = raw
		}
	}

	return values, errs
}
