// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package formengine

import (
	"strings"
	"testing"

	mi "github.com/ha1tch/minty"
	"github.com/ha1tch/xolu/pkg/client"
)

func render(fields []client.FieldDef, values Values, errs Errors, readOnly map[string]bool) string {
	return mi.RenderToString(RenderFields(fields, RenderOptions{Values: values, Errors: errs, ReadOnly: readOnly}))
}

func TestRenderFields_SelectOptionsRendersDropdownInsteadOfInput(t *testing.T) {
	fields := []client.FieldDef{{Name: "flavor", Type: "integer"}}
	html := mi.RenderToString(RenderFields(fields, RenderOptions{
		FieldOptions: map[string][]Option{
			"flavor": {{Key: "1", Value: "Chocolate"}, {Key: "2", Value: "Strawberry"}},
		},
	}))

	if !strings.Contains(html, "<select") {
		t.Fatalf("html = %q, want a <select>, not a plain number input", html)
	}
	if strings.Contains(html, `type="number"`) {
		t.Fatalf("html = %q, want no plain number input when field options are configured", html)
	}
	for _, want := range []string{`value="1"`, "Chocolate", `value="2"`, "Strawberry"} {
		if !strings.Contains(html, want) {
			t.Fatalf("html = %q, want it to contain %q", html, want)
		}
	}
}

func TestRenderFields_SelectMarksCurrentValueSelected(t *testing.T) {
	fields := []client.FieldDef{{Name: "flavor", Type: "integer"}}
	html := mi.RenderToString(RenderFields(fields, RenderOptions{
		Values:       Values{"flavor": float64(2)},
		FieldOptions: map[string][]Option{"flavor": {{Key: "1", Value: "Chocolate"}, {Key: "2", Value: "Strawberry"}}},
	}))

	// Find the whole <option ...> tag containing value="2" — minty
	// sorts attributes deterministically (alphabetically), so
	// "selected" can render before "value" within the tag; checking
	// the full tag rather than assuming an order avoids depending on
	// minty's internal attribute-ordering choice.
	valueIdx := strings.Index(html, `value="2"`)
	if valueIdx < 0 {
		t.Fatalf("html = %q, missing value=2 option", html)
	}
	tagStart := strings.LastIndex(html[:valueIdx], "<option")
	tagEnd := strings.Index(html[valueIdx:], ">")
	if tagStart < 0 || tagEnd < 0 {
		t.Fatalf("could not locate the full <option> tag around value=2 in %q", html)
	}
	tag := html[tagStart : valueIdx+tagEnd+1]
	if !strings.Contains(tag, "selected") {
		t.Fatalf("option tag = %q, want it marked selected", tag)
	}
}

func TestRenderFields_SelectOptionalFieldGetsEmptyChoice(t *testing.T) {
	fields := []client.FieldDef{{Name: "flavor", Type: "integer", Required: false}}
	html := mi.RenderToString(RenderFields(fields, RenderOptions{
		FieldOptions: map[string][]Option{"flavor": {{Key: "1", Value: "Chocolate"}}},
	}))
	if !strings.Contains(html, `<option value=""`) {
		t.Fatalf("html = %q, want an empty leading choice for an optional select field", html)
	}
}

func TestRenderFields_SelectRequiredFieldNoEmptyChoice(t *testing.T) {
	fields := []client.FieldDef{{Name: "flavor", Type: "integer", Required: true}}
	html := mi.RenderToString(RenderFields(fields, RenderOptions{
		FieldOptions: map[string][]Option{"flavor": {{Key: "1", Value: "Chocolate"}}},
	}))
	if strings.Contains(html, `<option value=""`) {
		t.Fatalf("html = %q, want no empty choice for a required select field", html)
	}
}

func TestRenderFields_SelectTakesPriorityOverFieldType(t *testing.T) {
	// A boolean field with configured options should still render as a
	// select, not a checkbox — FieldOptions overrides type-based dispatch.
	fields := []client.FieldDef{{Name: "status", Type: "boolean"}}
	html := mi.RenderToString(RenderFields(fields, RenderOptions{
		FieldOptions: map[string][]Option{"status": {{Key: "true", Value: "Active"}, {Key: "false", Value: "Inactive"}}},
	}))
	if !strings.Contains(html, "<select") {
		t.Fatalf("html = %q, want select to override the boolean-checkbox dispatch", html)
	}
	if strings.Contains(html, `type="checkbox"`) {
		t.Fatalf("html = %q, want no checkbox when field options are configured", html)
	}
}

func TestRenderFields_NoFieldOptionsUnaffected(t *testing.T) {
	// A field with no entry in FieldOptions renders exactly as before —
	// the zero-value RenderOptions (nil map) must not change anything.
	fields := []client.FieldDef{{Name: "count", Type: "integer"}}
	html := mi.RenderToString(RenderFields(fields, RenderOptions{}))
	if !strings.Contains(html, `type="number"`) {
		t.Fatalf("html = %q, want the normal number input when no FieldOptions are configured", html)
	}
}

func TestRenderFields_RefLinkRendersNavigationLink(t *testing.T) {
	fields := []client.FieldDef{{Name: "author_id", Type: "integer", Format: "ref"}}
	html := mi.RenderToString(RenderFields(fields, RenderOptions{
		Values:   Values{"author_id": float64(7)},
		RefLinks: map[string]RefLink{"author_id": {URL: "/connections/local/entities/users/7/edit", Label: "Alice"}},
	}))
	if !strings.Contains(html, `href="/connections/local/entities/users/7/edit"`) {
		t.Fatalf("html = %q, want the ref link's href", html)
	}
	if !strings.Contains(html, "Alice") {
		t.Fatalf("html = %q, want the resolved label", html)
	}
	// The raw-ID input must still be present and editable — a ref link
	// is an addition, not a replacement for direct ID editing.
	if !strings.Contains(html, `value="7"`) {
		t.Fatalf("html = %q, want the raw ID input still present", html)
	}
}

func TestRenderFields_NoRefLinkWhenNotConfigured(t *testing.T) {
	fields := []client.FieldDef{{Name: "author_id", Type: "integer", Format: "ref"}}
	html := mi.RenderToString(RenderFields(fields, RenderOptions{Values: Values{"author_id": float64(7)}}))
	if strings.Contains(html, "<a ") {
		t.Fatalf("html = %q, want no link when RefLinks has no entry for this field", html)
	}
}

func TestRenderFields_StringField(t *testing.T) {
	fields := []client.FieldDef{{Name: "title", Type: "string"}}
	html := render(fields, Values{"title": "Hello"}, nil, nil)

	for _, want := range []string{`type="text"`, `name="title"`, `value="Hello"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("html = %q, want it to contain %q", html, want)
		}
	}
}

func TestRenderFields_EmailFormat(t *testing.T) {
	fields := []client.FieldDef{{Name: "email", Type: "string", Format: "email"}}
	html := render(fields, nil, nil, nil)
	if !strings.Contains(html, `type="email"`) {
		t.Fatalf("html = %q, want type=email", html)
	}
}

func TestRenderFields_UnrecognisedFormatFallsBackToText(t *testing.T) {
	fields := []client.FieldDef{{Name: "x", Type: "string", Format: "something-unheard-of"}}
	html := render(fields, nil, nil, nil)
	if !strings.Contains(html, `type="text"`) {
		t.Fatalf("html = %q, want fallback to type=text for an unrecognised format", html)
	}
}

func TestRenderFields_RequiredField(t *testing.T) {
	fields := []client.FieldDef{{Name: "name", Type: "string", Required: true}}
	html := render(fields, nil, nil, nil)
	if !strings.Contains(html, `required="required"`) {
		t.Fatalf("html = %q, want the required attribute", html)
	}
	if !strings.Contains(html, "name *") {
		t.Fatalf("html = %q, want the label to mark the field required", html)
	}
}

func TestRenderFields_NotRequiredNoAsterisk(t *testing.T) {
	fields := []client.FieldDef{{Name: "nickname", Type: "string"}}
	html := render(fields, nil, nil, nil)
	if strings.Contains(html, `required=`) {
		t.Fatalf("html = %q, want no required attribute on an optional field", html)
	}
}

func TestRenderFields_IntegerField(t *testing.T) {
	fields := []client.FieldDef{{Name: "age", Type: "integer"}}
	html := render(fields, Values{"age": float64(42)}, nil, nil)
	for _, want := range []string{`type="number"`, `step="1"`, `value="42"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("html = %q, want it to contain %q", html, want)
		}
	}
}

func TestRenderFields_NumberFieldAllowsFractional(t *testing.T) {
	fields := []client.FieldDef{{Name: "score", Type: "number"}}
	html := render(fields, Values{"score": 3.5}, nil, nil)
	for _, want := range []string{`step="any"`, `value="3.5"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("html = %q, want it to contain %q", html, want)
		}
	}
}

func TestRenderFields_DecimalStaysText(t *testing.T) {
	// Decimal must never become <input type="number"> — browsers coerce
	// number inputs through float64, exactly the precision loss xolu's
	// decimal type exists to avoid.
	fields := []client.FieldDef{{Name: "price", Type: "string", Format: "decimal"}}
	html := render(fields, Values{"price": "19.999999999999999999"}, nil, nil)
	if !strings.Contains(html, `type="text"`) {
		t.Fatalf("html = %q, want decimal rendered as type=text", html)
	}
	if !strings.Contains(html, `value="19.999999999999999999"`) {
		t.Fatalf("html = %q, want the full-precision decimal string preserved verbatim", html)
	}
}

func TestRenderFields_BooleanCheckedWhenTrue(t *testing.T) {
	fields := []client.FieldDef{{Name: "active", Type: "boolean"}}
	html := render(fields, Values{"active": true}, nil, nil)
	if !strings.Contains(html, `type="checkbox"`) || !strings.Contains(html, "checked=") {
		t.Fatalf("html = %q, want a checked checkbox", html)
	}
}

func TestRenderFields_BooleanUncheckedWhenFalseOrAbsent(t *testing.T) {
	fields := []client.FieldDef{{Name: "active", Type: "boolean"}}

	htmlFalse := render(fields, Values{"active": false}, nil, nil)
	if strings.Contains(htmlFalse, "checked=") {
		t.Fatalf("html (false) = %q, want no checked attribute", htmlFalse)
	}

	htmlAbsent := render(fields, nil, nil, nil)
	if strings.Contains(htmlAbsent, "checked=") {
		t.Fatalf("html (absent) = %q, want no checked attribute", htmlAbsent)
	}
}

func TestRenderFields_DateTimeFormat(t *testing.T) {
	fields := []client.FieldDef{{Name: "starts_at", Type: "string", Format: "date-time"}}
	html := render(fields, nil, nil, nil)
	if !strings.Contains(html, `type="datetime-local"`) {
		t.Fatalf("html = %q, want type=datetime-local", html)
	}
}

func TestRenderFields_TimestampFormat(t *testing.T) {
	// xolu-specific format tag, same treatment as date-time.
	fields := []client.FieldDef{{Name: "seen_at", Type: "string", Format: "timestamp"}}
	html := render(fields, nil, nil, nil)
	if !strings.Contains(html, `type="datetime-local"`) {
		t.Fatalf("html = %q, want type=datetime-local for the timestamp format", html)
	}
}

func TestRenderFields_ObjectFieldFallsBackToJSONTextarea(t *testing.T) {
	fields := []client.FieldDef{{Name: "metadata", Type: "object"}}
	html := render(fields, Values{"metadata": map[string]any{"k": "v"}}, nil, nil)
	if !strings.Contains(html, "<textarea") {
		t.Fatalf("html = %q, want a <textarea> for an object field", html)
	}
	// The JSON is correctly HTML-escaped as textarea text content (quotes
	// become &#34;) — a browser decodes this back to the original JSON.
	if !strings.Contains(html, "k&#34;:&#34;v") {
		t.Fatalf("html = %q, want the object re-marshalled as JSON inside the textarea", html)
	}
}

func TestRenderFields_ArrayFieldFallsBackToJSONTextarea(t *testing.T) {
	fields := []client.FieldDef{{Name: "tags", Type: "array"}}
	html := render(fields, Values{"tags": []any{"a", "b"}}, nil, nil)
	if !strings.Contains(html, "<textarea") {
		t.Fatalf("html = %q, want a <textarea> for an array field", html)
	}
	if !strings.Contains(html, "a&#34;,&#34;b") {
		t.Fatalf("html = %q, want the array re-marshalled as JSON inside the textarea", html)
	}
}

func TestRenderFields_RefFieldByFormat(t *testing.T) {
	fields := []client.FieldDef{{Name: "author_id", Type: "integer", Format: "ref"}}
	html := render(fields, Values{"author_id": float64(7)}, nil, nil)
	if !strings.Contains(html, `value="7"`) {
		t.Fatalf("html = %q, want the ref's target ID as the value", html)
	}
}

func TestRenderFields_RefFieldByType(t *testing.T) {
	// Defensive: FieldDef.Type's doc comment allows "ref" as a Type
	// value too, even though the confirmed extraction path only ever
	// sets it via Format. Both must be handled the same way.
	fields := []client.FieldDef{{Name: "author_id", Type: "ref"}}
	html := render(fields, Values{"author_id": float64(7)}, nil, nil)
	if !strings.Contains(html, `value="7"`) {
		t.Fatalf("html = %q, want the ref's target ID as the value", html)
	}
}

func TestRenderFields_ErrorRendersInline(t *testing.T) {
	fields := []client.FieldDef{{Name: "email", Type: "string"}}
	html := render(fields, nil, Errors{"email": "not a valid address"}, nil)
	if !strings.Contains(html, "not a valid address") {
		t.Fatalf("html = %q, want the error message present", html)
	}
}

func TestRenderFields_NoErrorNoErrorNode(t *testing.T) {
	fields := []client.FieldDef{{Name: "email", Type: "string"}}
	html := render(fields, nil, Errors{}, nil)
	if strings.Contains(html, "text-red") {
		t.Fatalf("html = %q, want no error styling when there is no error", html)
	}
}

func TestRenderFields_ReadOnlyDisablesField(t *testing.T) {
	fields := []client.FieldDef{{Name: "sku", Type: "string"}}
	html := render(fields, Values{"sku": "ABC"}, nil, map[string]bool{"sku": true})
	if !strings.Contains(html, `disabled="disabled"`) {
		t.Fatalf("html = %q, want the field disabled", html)
	}
}

func TestRenderFields_NotListedInReadOnlyStaysEnabled(t *testing.T) {
	fields := []client.FieldDef{{Name: "sku", Type: "string"}}
	html := render(fields, nil, nil, map[string]bool{"other_field": true})
	if strings.Contains(html, `disabled=`) {
		t.Fatalf("html = %q, want sku enabled since it isn't in readOnly", html)
	}
}

func TestRenderFields_MultipleFieldsRenderInGivenOrder(t *testing.T) {
	fields := []client.FieldDef{
		{Name: "zebra", Type: "string"},
		{Name: "alpha", Type: "string"},
	}
	html := render(fields, nil, nil, nil)
	if strings.Index(html, "zebra") > strings.Index(html, "alpha") {
		t.Fatalf("html = %q, want fields in the given order (schema order), not resorted", html)
	}
}

func TestRenderFields_URLFormat(t *testing.T) {
	fields := []client.FieldDef{{Name: "website", Type: "string", Format: "url"}}
	html := render(fields, nil, nil, nil)
	if !strings.Contains(html, `type="url"`) {
		t.Fatalf("html = %q, want type=url", html)
	}
}

func TestRenderFields_DisabledCheckbox(t *testing.T) {
	fields := []client.FieldDef{{Name: "active", Type: "boolean"}}
	html := render(fields, Values{"active": true}, nil, map[string]bool{"active": true})
	if !strings.Contains(html, `disabled="disabled"`) {
		t.Fatalf("html = %q, want the checkbox disabled", html)
	}
}

func TestStringifyValue_UnmarshalableFallsBackToSprintf(t *testing.T) {
	// A Go channel value can never be JSON-marshalled — exercises
	// stringifyValue's fmt.Sprintf fallback rather than losing the
	// field's content on a marshal error.
	got := stringifyValue(make(chan int))
	if !strings.HasPrefix(got, "0x") && !strings.Contains(got, "chan") {
		t.Fatalf("stringifyValue(chan) = %q, want a %%v-style fallback string", got)
	}
}

func TestStringifyValue(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"true", true, "true"},
		{"false", false, "false"},
		{"whole float", float64(42), "42"},
		{"fractional float", 3.14, "3.14"},
		{"object", map[string]any{"a": float64(1)}, `{"a":1}`},
		{"array", []any{float64(1), float64(2)}, `[1,2]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := stringifyValue(c.in); got != c.want {
				t.Fatalf("stringifyValue(%#v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestValues_GetOnNilIsSafe(t *testing.T) {
	var v Values
	if got := v.Get("anything"); got != nil {
		t.Fatalf("Get on nil Values = %v, want nil", got)
	}
}
