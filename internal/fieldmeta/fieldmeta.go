// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package fieldmeta implements listbox/select field configuration as
// xoluman-owned bookkeeping documents inside the target xolu instance —
// the same pattern already established for T-09's blob-folder entities:
// a plain entity type (xoluman_field_meta), created schema-lessly
// (confirmed: xolu accepts writes to an unregistered entity type — "if
// no schema is registered, all writes pass"), not a new xolu primitive.
// No new xolu API is needed for any of this.
//
// One document per (entity_type, field_name) pair says a field should
// render as a dropdown instead of its default input, and where its
// options come from — embedded directly ("static") or looked up from
// another entity's own options-shaped field ("ref").
//
// Because it's just another entity type, the metadata itself is
// manageable through the existing generic entity browser with zero new
// UI code — browse to xoluman_field_meta and create/edit rows there.
package fieldmeta

import (
	"context"
	"fmt"

	"github.com/ha1tch/xolu/pkg/client"
)

// EntityType is the bookkeeping entity type name field-meta documents
// are stored under.
const EntityType = "xoluman_field_meta"

// defaultRefOptionsField is which field on a ref-sourced document holds
// its own options list, when Meta.RefOptionsField is left empty.
const defaultRefOptionsField = "options"

// maxFieldMetaRows bounds a single LoadForEntityType fetch — a
// reasonable ceiling for a bookkeeping table (one row per configured
// dropdown field across an entire xolu instance). Not paginated further
// in v1; documented as a limitation, not silently capped without
// saying so.
const maxFieldMetaRows = 500

// Option is one key/value choice. Represented uniformly for both
// integer- and string-typed target fields — for a string field, Key
// conventionally equals Value (the convention this package expects
// whoever fills in a xoluman_field_meta document to follow, since
// there's no separate xolu-side validation of it).
type Option struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Meta is one field's dropdown configuration.
type Meta struct {
	ID              int64    `json:"id,omitempty"`
	EntityType      string   `json:"entity_type"`
	FieldName       string   `json:"field_name"`
	OptionKind      string   `json:"option_kind"`       // "static" or "ref"
	Options         []Option `json:"options,omitempty"` // OptionKind == "static"
	RefEntity       string   `json:"ref_entity,omitempty"`
	RefID           int64    `json:"ref_id,omitempty"`
	RefOptionsField string   `json:"ref_options_field,omitempty"`
}

// LoadForEntityType fetches every xoluman_field_meta document and
// returns the ones matching entityType, keyed by field name.
// Client.List has no filter parameter (confirmed against
// pkg/client/client.go: Limit/Offset/Sort only) — filtering happens
// here in Go rather than via a hand-built OQL WHERE clause, which is
// both simpler and avoids constructing query strings from data at all.
// A xoluman_field_meta entity type that hasn't been created yet (no
// rows, possibly no schema) is not an error — returns an empty map.
func LoadForEntityType(ctx context.Context, c *client.Client, entityType string) (map[string]Meta, error) {
	result, err := c.List(ctx, EntityType, &client.ListParams{Limit: maxFieldMetaRows})
	if err != nil {
		if xoluErr, ok := err.(*client.Error); ok && xoluErr.HTTPStatus == 404 {
			return map[string]Meta{}, nil // entity type doesn't exist yet — no field metadata configured anywhere
		}
		return nil, err
	}

	out := make(map[string]Meta)
	for _, e := range result.Entities {
		m, err := decodeMeta(e.Data)
		if err != nil {
			continue // a malformed row shouldn't break every other field's rendering — skip it
		}
		if m.EntityType != entityType {
			continue
		}
		out[m.FieldName] = m
	}
	return out, nil
}

// decodeMeta converts a raw entity document into a Meta, tolerating the
// options array's "key" values coming back as either JSON numbers
// (float64, from json.Unmarshal's default decoding) or strings —
// whoever filled in the document typed raw JSON into a textarea
// (formengine's own array-field rendering), so either is plausible
// depending on whether they quoted the key.
func decodeMeta(doc map[string]any) (Meta, error) {
	m := Meta{
		EntityType: str(doc["entity_type"]),
		FieldName:  str(doc["field_name"]),
		OptionKind: str(doc["option_kind"]),
		RefEntity:  str(doc["ref_entity"]),
		RefOptionsField: func() string {
			if v := str(doc["ref_options_field"]); v != "" {
				return v
			}
			return defaultRefOptionsField
		}(),
	}
	if idVal, ok := doc["id"].(float64); ok {
		m.ID = int64(idVal)
	}
	if refIDVal, ok := doc["ref_id"].(float64); ok {
		m.RefID = int64(refIDVal)
	}
	if m.EntityType == "" || m.FieldName == "" {
		return Meta{}, fmt.Errorf("fieldmeta: row missing entity_type or field_name")
	}

	if raw, ok := doc["options"]; ok {
		opts, err := decodeOptions(raw)
		if err != nil {
			return Meta{}, err
		}
		m.Options = opts
	}
	return m, nil
}

// decodeOptions parses an "options" field value (a JSON array of
// {key, value} objects) into []Option, coercing a numeric key to its
// string form.
func decodeOptions(raw any) ([]Option, error) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("fieldmeta: options is not an array")
	}
	out := make([]Option, 0, len(arr))
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		var key string
		switch k := obj["key"].(type) {
		case string:
			key = k
		case float64:
			key = formatNumericKey(k)
		}
		value := str(obj["value"])
		if value == "" {
			continue
		}
		if key == "" {
			key = value // string-field convention: key defaults to value when absent
		}
		out = append(out, Option{Key: key, Value: value})
	}
	return out, nil
}

// formatNumericKey renders a JSON-number key as an integer string when
// it has no fractional part (the overwhelmingly common case — option
// keys are integer IDs), falling back to Go's default float formatting
// otherwise rather than silently truncating a genuinely fractional key.
func formatNumericKey(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%v", f)
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// ResolveOptions returns m's actual option list: Options directly for
// "static", or fetched from the referenced document's RefOptionsField
// for "ref". A ref-sourced Meta with a fetch failure or a malformed
// target document returns an error rather than silently rendering an
// empty dropdown, which would look like "no choices exist" instead of
// "something is misconfigured."
func ResolveOptions(ctx context.Context, c *client.Client, m Meta) ([]Option, error) {
	if m.OptionKind != "ref" {
		return m.Options, nil
	}

	entity, err := c.Get(ctx, m.RefEntity, m.RefID)
	if err != nil {
		return nil, fmt.Errorf("fieldmeta: fetching ref options source %s/%d: %w", m.RefEntity, m.RefID, err)
	}

	raw, ok := entity.Data[m.RefOptionsField]
	if !ok {
		return nil, fmt.Errorf("fieldmeta: %s/%d has no %q field", m.RefEntity, m.RefID, m.RefOptionsField)
	}
	return decodeOptions(raw)
}

// RememberRefTarget saves the entity type a person specified for a ref
// field whose schema doesn't declare one — so the next time anyone
// edits that same field, on any row, it's known without asking again.
// Same bookkeeping-entity mechanism as the rest of this package, a new
// OptionKind ("ref-target") rather than overloading "ref" (which means
// something different: where a *select* field's options come from, not
// a ref field's own target).
//
// Real gap this closes: xolu's embedded read-shape for a ref value
// doesn't self-identify its target entity type (confirmed directly,
// not assumed), so there was no way to pre-fill the companion
// entity-type input on a plain read — every edit of an undeclared-
// target ref field required re-typing the entity type, which is what
// was actually blocking saves in practice. Once remembered here, it's
// treated the same as a schema-declared target from then on.
//
// Idempotent in spirit, not in mechanism: writes a new document each
// call rather than checking for and updating an existing one (this
// package has no update-if-exists path yet, and Client.List has no
// filter to check cheaply) — callers should only call this when the
// value actually differs from what LookupRememberedTarget already
// returns, not on every single save.
func RememberRefTarget(ctx context.Context, c *client.Client, entityType, fieldName, targetEntity string) error {
	_, err := c.Create(ctx, EntityType, map[string]any{
		"entity_type": entityType,
		"field_name":  fieldName,
		"option_kind": "ref-target",
		"ref_entity":  targetEntity,
	})
	return err
}

// LookupRememberedTarget returns the remembered target entity type for
// fieldName from an already-loaded metas map (LoadForEntityType), if
// one was saved via RememberRefTarget. Distinct from a "ref"-kind
// Meta's own RefEntity, which means something else (where a select
// field's options come from) — only a "ref-target" kind counts here.
func LookupRememberedTarget(metas map[string]Meta, fieldName string) (string, bool) {
	m, ok := metas[fieldName]
	if !ok || m.OptionKind != "ref-target" || m.RefEntity == "" {
		return "", false
	}
	return m.RefEntity, true
}
