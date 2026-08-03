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
		id, ok := refTargetID(values.Get(ref.Name))
		if !ok {
			continue // unset ref field — nothing to link to yet
		}
		opts.RefLinks[ref.Name] = formengine.RefLink{
			URL:   fmt.Sprintf("/connections/%s/entities/%s/%d/edit", connName, ref.Target, id),
			Label: resolveRefLabel(ctx, c, ref.Target, id),
		}
	}

	return opts
}

// refTargetID extracts an int64 ID from a ref field's raw decoded-JSON
// value (float64, per encoding/json's default shapes) or a numeric
// string (formengine.Values coming from a re-displayed, not-yet-saved
// form uses ParseFormValues' own float64 output for ref fields, but a
// freshly-typed value that failed some other field's validation could
// in principle still be a string — handled defensively either way).
// ok is false for zero, absent, or unparseable values.
func refTargetID(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		if t == 0 {
			return 0, false
		}
		return int64(t), true
	case string:
		var id int64
		if _, err := fmt.Sscanf(t, "%d", &id); err != nil || id == 0 {
			return 0, false
		}
		return id, true
	default:
		return 0, false
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
