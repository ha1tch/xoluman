// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"context"
	"fmt"

	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/fieldmeta"
	"github.com/ha1tch/xoluman/internal/formengine"
)

// resolveFormOptionsMaybeSchema wraps resolveFormOptions for callers
// that may only have an inferred field list, not a real schema (see
// resolveEntityFields) — constructs a minimal synthetic schema (name
// only, no Refs) when schema is nil, since resolveFormOptions itself
// needs something to read .Name from for the fieldmeta lookup.
// Ref-link resolution is simply skipped for an inferred field list —
// inference has no way to know which fields are references.
func resolveFormOptionsMaybeSchema(ctx context.Context, c *xclient.Client, connName, entityType string, schema *xclient.EntitySchema, values formengine.Values, errs formengine.Errors) formengine.RenderOptions {
	if schema == nil {
		schema = &xclient.EntitySchema{Name: entityType}
	}
	return resolveFormOptions(ctx, c, connName, schema, values, errs)
}

// refLabelFields are tried in order when resolving a human-readable
// label for a ref target — the first present, non-empty string field
// wins. There's no formal "display field" designation on a xolu schema
// (a real gap, flagged in docs/xolu-requests.md's reviewed-but-not-
// requested notes); this is a stated, revisable heuristic, not a
// discovered convention.
var refLabelFields = []string{"name", "title", "label"}

// resolveFormOptions builds the formengine.RenderOptions extras for an
// entity form: listbox/select options from any configured
// xoluman_field_meta rows, and ref navigation links for the current
// values of any ref fields on schema.
//
// Failures here degrade gracefully rather than breaking the form: a
// field-options or ref-label lookup failing means that one enhancement
// is skipped, not that editing stops working. The base editable input
// (raw value, or raw ref ID) still renders regardless.
func resolveFormOptions(ctx context.Context, c *xclient.Client, connName string, schema *xclient.EntitySchema, values formengine.Values, errs formengine.Errors) formengine.RenderOptions {
	opts := formengine.RenderOptions{
		Values:       values,
		Errors:       errs,
		FieldOptions: map[string][]formengine.Option{},
		RefLinks:     map[string]formengine.RefLink{},
	}

	if metas, err := fieldmeta.LoadForEntityType(ctx, c, schema.Name); err == nil {
		for fieldName, m := range metas {
			resolved, err := fieldmeta.ResolveOptions(ctx, c, m)
			if err != nil {
				continue // one misconfigured dropdown shouldn't break the rest of the form
			}
			fieldOpts := make([]formengine.Option, len(resolved))
			for i, o := range resolved {
				fieldOpts[i] = formengine.Option{Key: o.Key, Value: o.Value}
			}
			opts.FieldOptions[fieldName] = fieldOpts
		}
	}

	for _, ref := range schema.Refs {
		if ref.Target == "" {
			continue // polymorphic target — no single entity type to link to
		}
		id, embeddedLabel, ok := refValueInfo(values.Get(ref.Name))
		if !ok {
			continue // unset ref field — nothing to link to yet
		}
		label := embeddedLabel
		if label == "" {
			// GET normally embeds the resolved target document
			// directly (see refValueInfo's own doc), so this fetch is
			// the exception, not the rule — it only runs when the
			// value is a bare ID: a freshly-typed, not-yet-saved form
			// field, or a schema-less/inferred context with no
			// embedded document to read a label from.
			label = resolveRefLabel(ctx, c, ref.Target, id)
		}
		opts.RefLinks[ref.Name] = formengine.RefLink{
			URL:   fmt.Sprintf("/connections/%s/entities/%s/%d/edit", connName, ref.Target, id),
			Label: label,
		}
	}

	return opts
}

// refValueInfo extracts what's known about a ref field's raw decoded-
// JSON value. There are three real shapes to handle, confirmed
// directly against real xolu, not assumed:
//   - a bare numeric ID (float64, or a numeric string from a
//     redisplayed-after-error form) — what a freshly-typed, not-yet-
//     submitted value looks like, and what this package's own text
//     input for a ref field expects to show/accept
//   - xolu's write shape: {"type":"REF","entity":"...","id":N} — what
//     a ref field must be submitted as; a bare integer is rejected
//     with XOLU-VL001 ("expected {type,entity,id}, got float64"),
//     confirmed by trying it directly against a real server
//   - xolu's read shape: GET embeds the *entire resolved target
//     document* in place of the reference — {"id":N,"name":"...",
//     ...every other field the target document has}, not a stub with
//     just an ID. This is not documented anywhere consulted before
//     this was caught by an end-to-end write-then-read round trip
//     against a real server; every earlier ref-handling assumption in
//     this package (link extraction, form redisplay, list preview) was
//     built against a bare-ID assumption that never actually matched
//     what GET returns.
//
// Returns the target ID and, when the value is the read-embedded shape
// and carries one of refLabelFields, that label directly — no extra
// fetch needed, since xolu already did the resolving. ok is false for
// zero, absent, or unparseable values.
func refValueInfo(v any) (id int64, embeddedLabel string, ok bool) {
	switch t := v.(type) {
	case float64:
		if t == 0 {
			return 0, "", false
		}
		return int64(t), "", true
	case string:
		var parsedID int64
		if _, err := fmt.Sscanf(t, "%d", &parsedID); err != nil || parsedID == 0 {
			return 0, "", false
		}
		return parsedID, "", true
	case map[string]any:
		idVal, ok := t["id"].(float64)
		if !ok || idVal == 0 {
			return 0, "", false
		}
		for _, field := range refLabelFields {
			if s, ok := t[field].(string); ok && s != "" {
				return int64(idVal), s, true
			}
		}
		return int64(idVal), "", true
	default:
		return 0, "", false
	}
}

// resolveRefLabel fetches the target entity and picks the first
// present field from refLabelFields as a human-readable label, falling
// back to "type #id" when none is present or the fetch fails — a
// broken or stale reference should still be visible and navigable
// (clicking it will show the real error), not silently hidden.
func resolveRefLabel(ctx context.Context, c *xclient.Client, targetType string, id int64) string {
	entity, err := c.Get(ctx, targetType, id)
	if err != nil {
		return fmt.Sprintf("%s #%d", targetType, id)
	}
	for _, field := range refLabelFields {
		if v, ok := entity.Data[field].(string); ok && v != "" {
			return v
		}
	}
	return fmt.Sprintf("%s #%d", targetType, id)
}
