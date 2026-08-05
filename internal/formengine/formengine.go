// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package formengine renders a flat, schema-driven form from xolu's own
// JSON Schema field extraction (client.FieldDef), for arbitrary entity
// types on arbitrary xolu instances. Deliberately not Seam's formengine
// (github.com/ha1tch/seam-ui/internal/formengine): no tabs, no
// x-seam-relation, no visibility rules — those are Seam's asset-model
// concerns, not xoluman's. This package renders flat input rows only,
// in the order xolu's own schema declares them.
//
// This package does not import internal/ui — internal/ui imports this
// package instead (formengine is the reusable rendering logic; ui wires
// it into actual pages). Its own small set of Tailwind classes
// deliberately duplicates ui's form styling rather than depending on
// ui's unexported constants, to keep the import direction one-way.
package formengine

import (
	"encoding/json"
	"fmt"
	"strconv"

	mi "github.com/ha1tch/minty"
	"github.com/ha1tch/xolu/pkg/client"
)

// Values holds current field values for a form — typically an
// Entity.Data map for an edit form, or nil for a create form.
type Values map[string]any

// Get returns the value for name, or nil if absent. Safe to call on a
// nil Values.
func (v Values) Get(name string) any {
	if v == nil {
		return nil
	}
	return v[name]
}

// Errors maps field name to a validation error message, shown inline
// beneath that field.
type Errors map[string]string

const (
	labelClass   = "block mt-3 mb-1 text-sm text-gray-600 dark:text-gray-400"
	inputClass   = "w-full max-w-md px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 text-sm disabled:bg-gray-100 dark:disabled:bg-gray-900 disabled:text-gray-500"
	errorClass   = "text-red-600 dark:text-red-400 text-xs mt-1"
	checkClass   = "mt-3 h-4 w-4"
	refLinkClass = "block mt-1 text-xs text-indigo-600 dark:text-indigo-400 hover:underline"
)

// Option is one <select> choice — the listbox/select convention
// (xoluman_field_meta documents, resolved by internal/fieldmeta). Key
// is what's submitted (and, for an integer-typed field, what
// ParseFormValues coerces back to a number — no changes needed there,
// a <select> submits its value the same as any other input); Value is
// the visible label.
type Option struct {
	Key   string
	Value string
}

// RefLink points a ref field's rendered navigation link at its current
// target — resolved by the caller (internal/ui), which has the
// connection/entity-type context this package deliberately doesn't
// know about.
type RefLink struct {
	URL   string
	Label string
}

// RenderOptions configures RenderFields beyond the basic field/value/
// error rendering. The zero value renders every field with its default
// input and no ref links — existing behaviour, unchanged.
type RenderOptions struct {
	Values   Values
	Errors   Errors
	ReadOnly map[string]bool

	// FieldOptions renders a <select> instead of a field's normal
	// input when present for that field name, regardless of the
	// field's underlying JSON Schema type — the listbox/select
	// convention. A field type still governs how the selected value
	// is coerced back on submit (ParseFormValues' existing dispatch;
	// this package makes no change there).
	FieldOptions map[string][]Option

	// RefLinks adds a "→ view" navigation link after a ref field's
	// input when present for that field name. The field's raw-ID
	// input itself is unchanged — editing a reference by ID and
	// navigating to see what it currently points at are two different
	// actions, both available side by side.
	RefLinks map[string]RefLink

	// RefTargets names the target entity type for a ref field, when
	// the schema declares one. A field present here renders as a
	// single ID input, same as always. A ref field ABSENT here (no
	// entry at all, not even empty) is the real gap this closes: many
	// real schemas declare a ref field as {"format":"ref"} with no
	// "target" at all — confirmed directly, this is what examples/
	// crm's own seed script does for every ref field except users' —
	// and xolu itself has no way to accept a write for such a field
	// without being told which entity type it points to; there is no
	// way to infer this from the schema or from a read response
	// (checked directly: the embedded read-shape doesn't self-
	// identify its own entity type either). Previously this silently
	// fell back to submitting a bare ID, which xolu correctly and
	// unconditionally rejects — every create or update touching such
	// a field failed, which was the real, unresolved core of "saving
	// doesn't work." refInput renders a companion entity-type input
	// for exactly these fields now, and ParseFormValues uses it when
	// present.
	RefTargets map[string]string
}

// RenderFields renders one labeled input row per field, in the order
// given — which matches xolu's own JSON Schema property order, since
// that's the order client.GetEntitySchema's field extraction preserves.
func RenderFields(fields []client.FieldDef, opts RenderOptions) mi.H {
	return func(b *mi.Builder) mi.Node {
		rows := make([]interface{}, 0, len(fields))
		for _, f := range fields {
			rows = append(rows, renderField(b, f, opts))
		}
		return b.Div(rows...)
	}
}

func renderField(b *mi.Builder, f client.FieldDef, opts RenderOptions) mi.Node {
	values := opts.Values
	disabled := opts.ReadOnly[f.Name]
	label := f.Name
	if f.Required {
		label += " *"
	}

	var input mi.Node
	var refLink interface{}
	switch {
	case len(opts.FieldOptions[f.Name]) > 0:
		// Listbox/select convention takes priority over every other
		// dispatch below — a field with configured options renders as
		// a dropdown regardless of its underlying JSON Schema type.
		input = selectInput(b, f, values, disabled, opts.FieldOptions[f.Name])
	case f.Format == "ref" || f.Type == "ref":
		_, targetKnown := opts.RefTargets[f.Name]
		input = refInput(b, f, values, disabled, targetKnown)
		if rl, ok := opts.RefLinks[f.Name]; ok {
			// The bolt icon opens rl.URL in a modal instead of a full
			// navigation — self-contained here rather than calling into
			// internal/ui (which imports this package, so the reverse
			// would be an import cycle); ui.RefJumpButton renders the
			// identical markup for the list-preview's own ref links.
			refLink = b.Span(
				b.A(mi.Href(rl.URL), mi.Class(refLinkClass), "→ "+rl.Label),
				b.Button(
					mi.Type("button"),
					mi.Class("inline-flex items-center ml-1 text-gray-400 dark:text-gray-500 hover:text-indigo-600 dark:hover:text-indigo-400 cursor-pointer border-0 bg-transparent p-0 align-middle"),
					mi.Attr("title", "Open in a modal"),
					mi.HxGet(rl.URL), mi.HxTarget("#modal-body"), mi.HxSwap("innerHTML"),
					mi.Attr("onclick", "XModal.open('Loading…')"),
					mi.Raw(`<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>`),
				),
			)
		}
	case f.Type == "boolean":
		input = checkboxInput(b, f, values, disabled)
	case f.Type == "integer" || f.Type == "number":
		input = numberInput(b, f, values, disabled)
	case f.Format == "decimal":
		// Rendered as text, not <input type="number">: browsers coerce
		// number inputs through float64, which is exactly the precision
		// loss xolu's decimal type exists to avoid. The value travels
		// as a plain string all the way to submission.
		input = textInput(b, f, values, disabled)
	case f.Format == "date-time" || f.Format == "timestamp":
		input = dateTimeInput(b, f, values, disabled)
	case f.Type == "object" || f.Type == "array":
		input = jsonTextarea(b, f, values, disabled)
	default: // "string" and anything else unrecognised
		input = textInput(b, f, values, disabled)
	}

	var errNode interface{}
	if msg := opts.Errors[f.Name]; msg != "" {
		errNode = b.Div(mi.Class(errorClass), msg)
	}

	if f.Type == "boolean" {
		// Checkbox layout reads better label-after-input than the
		// label-above-input pattern every other field uses.
		return b.Div(
			b.Label(mi.Class(labelClass+" flex items-center gap-2"), input, label),
			errNode,
		)
	}

	return b.Div(
		b.Label(mi.For(f.Name), mi.Class(labelClass), label),
		input,
		refLink,
		errNode,
	)
}

// selectInput renders a <select> from a resolved option list. An empty
// leading choice is offered for optional fields — an optional dropdown
// needs a way to submit "nothing chosen," the same as an optional text
// input being left blank.
func selectInput(b *mi.Builder, f client.FieldDef, values Values, disabled bool, options []Option) mi.Node {
	current := stringifyValue(values.Get(f.Name))

	attrs := []interface{}{mi.ID(f.Name), mi.Name(f.Name), mi.Class(inputClass)}
	if f.Required {
		attrs = append(attrs, mi.Required())
	}
	if disabled {
		attrs = append(attrs, mi.Attr("disabled", "disabled"))
	}

	if !f.Required {
		attrs = append(attrs, b.Option(mi.Value(""), ""))
	}
	for _, opt := range options {
		optAttrs := []interface{}{mi.Value(opt.Key)}
		if opt.Key == current {
			optAttrs = append(optAttrs, mi.Selected())
		}
		optAttrs = append(optAttrs, opt.Value)
		attrs = append(attrs, b.Option(optAttrs...))
	}
	return b.Select(attrs...)
}

func baseAttrs(f client.FieldDef, disabled bool) []mi.Attribute {
	attrs := []mi.Attribute{mi.ID(f.Name), mi.Name(f.Name), mi.Class(inputClass)}
	if f.Required {
		attrs = append(attrs, mi.Required())
	}
	if disabled {
		attrs = append(attrs, mi.Attr("disabled", "disabled"))
	}
	return attrs
}

func textInput(b *mi.Builder, f client.FieldDef, values Values, disabled bool) mi.Node {
	attrs := append([]mi.Attribute{mi.Type(inputTypeFor(f.Format))}, baseAttrs(f, disabled)...)
	attrs = append(attrs, mi.Value(stringifyValue(values.Get(f.Name))))
	return b.Input(attrs...)
}

// refInput renders a ref field's editable input showing just the
// target ID — not the generic stringifyValue, which would json.Marshal
// an entire embedded object into the field. xolu's GET response embeds
// the *whole resolved target document* in place of a reference
// ({"id":N,"name":"...",...every other field the target has}), not a
// bare ID or the write-shape structured object either — confirmed
// directly against a real server, not assumed. refFieldValue extracts
// just the ID regardless of which of the three real shapes (bare
// number, xolu's write shape, xolu's read shape) the value happens to
// be in.
//
// When targetKnown is false, a companion entity-type text input is
// rendered alongside the ID input (name: f.Name+"__ref_entity") — see
// RefTargets' own doc comment for why this exists: a real, previously
// unsolved gap where such a field could never be written at all.
func refInput(b *mi.Builder, f client.FieldDef, values Values, disabled bool, targetKnown bool) mi.Node {
	attrs := append([]mi.Attribute{mi.Type("text")}, baseAttrs(f, disabled)...)
	attrs = append(attrs, mi.Value(refFieldValue(values.Get(f.Name))))
	idInput := b.Input(attrs...)
	if targetKnown {
		return idInput
	}

	entityAttrs := []mi.Attribute{
		mi.Type("text"), mi.ID(f.Name + "__ref_entity"), mi.Name(f.Name + "__ref_entity"),
		mi.Class(inputClass), mi.Attr("placeholder", "entity type, e.g. users"),
		mi.Value(refFieldEntity(values.Get(f.Name))),
	}
	if disabled {
		entityAttrs = append(entityAttrs, mi.Attr("disabled", "disabled"))
	}
	if f.Required {
		entityAttrs = append(entityAttrs, mi.Required())
	}
	return b.Div(mi.Class("flex gap-2"),
		b.Div(mi.Class("flex-1"),
			b.Label(mi.Class("block text-xs text-gray-500 dark:text-gray-500 mb-0.5"), "entity type"),
			b.Input(entityAttrs...),
		),
		b.Div(mi.Class("flex-1"),
			b.Label(mi.Class("block text-xs text-gray-500 dark:text-gray-500 mb-0.5"), "id"),
			idInput,
		),
	)
}

// refFieldValue extracts a ref field's target ID as a plain string,
// regardless of which real shape the raw decoded-JSON value is in. See
// refInput's doc comment for why this can't just be stringifyValue.
func refFieldValue(v any) string {
	switch t := v.(type) {
	case float64:
		if t == 0 {
			return ""
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case string:
		return t // already a plain numeric string (e.g. redisplay after a validation error elsewhere in the form)
	case map[string]any:
		if id, ok := t["id"].(float64); ok {
			return strconv.FormatFloat(id, 'f', -1, 64)
		}
		return ""
	default:
		return ""
	}
}

// refFieldEntity extracts a ref field's target entity type from the
// write-shape ({"type":"REF","entity":"...","id":N}) when the current
// value happens to be in that shape — e.g. redisplaying a form after
// a validation error, where ParseFormValues already built the write
// shape from what was submitted. Returns "" for every other shape
// (xolu's read-shape embeds the target's own document, which does not
// self-identify its entity type — confirmed directly, not assumed),
// which is the honest, correct answer: there is nothing to prefill
// from a plain read.
func refFieldEntity(v any) string {
	if m, ok := v.(map[string]any); ok {
		if entity, ok := m["entity"].(string); ok {
			return entity
		}
	}
	return ""
}

// inputTypeFor maps a JSON Schema "format" to the closest native HTML
// input type. Unrecognised or empty formats fall back to plain text
// rather than guessing.
func inputTypeFor(format string) string {
	switch format {
	case "email":
		return "email"
	case "uri", "url":
		return "url"
	default:
		return "text"
	}
}

func numberInput(b *mi.Builder, f client.FieldDef, values Values, disabled bool) mi.Node {
	attrs := append([]mi.Attribute{mi.Type("number")}, baseAttrs(f, disabled)...)
	if f.Type == "integer" {
		attrs = append(attrs, mi.Step("1"))
	} else {
		attrs = append(attrs, mi.Step("any"))
	}
	attrs = append(attrs, mi.Value(stringifyValue(values.Get(f.Name))))
	return b.Input(attrs...)
}

func dateTimeInput(b *mi.Builder, f client.FieldDef, values Values, disabled bool) mi.Node {
	attrs := append([]mi.Attribute{mi.Type("datetime-local")}, baseAttrs(f, disabled)...)
	attrs = append(attrs, mi.Value(stringifyValue(values.Get(f.Name))))
	return b.Input(attrs...)
}

// checkboxInput's disabled+unchecked submission gotcha: browsers omit an
// unchecked checkbox from form data entirely, not send false — whatever
// handles the submitted form must treat the field's absence as false,
// not skip updating it. Not this package's concern (it only renders),
// but worth knowing before wiring a submit handler against this output.
func checkboxInput(b *mi.Builder, f client.FieldDef, values Values, disabled bool) mi.Node {
	attrs := []mi.Attribute{mi.Type("checkbox"), mi.ID(f.Name), mi.Name(f.Name), mi.Class(checkClass)}
	if disabled {
		attrs = append(attrs, mi.Attr("disabled", "disabled"))
	}
	if truthy(values.Get(f.Name)) {
		attrs = append(attrs, mi.Checked())
	}
	return b.Input(attrs...)
}

func truthy(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

// jsonTextarea is the fallback for "object" and "array" fields — flat
// fields, JSON-Schema types only means these aren't decomposed into
// nested sub-forms; the raw JSON is directly editable instead. Honest
// about the limitation rather than silently dropping the field.
// Textarea (unlike Input) is not self-closing, so it takes ...interface{}
// (attributes plus its text content), not ...Attribute like the Input
// helpers above — baseAttrs' []mi.Attribute needs converting.
func jsonTextarea(b *mi.Builder, f client.FieldDef, values Values, disabled bool) mi.Node {
	base := baseAttrs(f, disabled)
	args := make([]interface{}, 0, len(base)+2)
	args = append(args, mi.Rows(6))
	for _, a := range base {
		args = append(args, a)
	}
	args = append(args, stringifyValue(values.Get(f.Name)))
	return b.Textarea(args...)
}

// stringifyValue renders a decoded-JSON value (string, float64, bool,
// map, slice, or nil, per encoding/json's default unmarshal-into-any
// shapes) as the string an HTML input/textarea needs. Objects and
// arrays are re-marshalled to JSON text; anything that fails to
// marshal falls back to fmt.Sprintf rather than losing the field's
// content entirely.
func stringifyValue(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}
