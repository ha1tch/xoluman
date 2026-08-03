// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package importer

import (
	"strings"
	"testing"

	"github.com/ha1tch/xolu/pkg/client"
)

func widgetFields() []client.FieldDef {
	return []client.FieldDef{
		{Name: "name", Type: "string", Required: true},
		{Name: "count", Type: "integer"},
		{Name: "active", Type: "boolean"},
	}
}

func TestParseCSV_ValidRows(t *testing.T) {
	csv := "name,count,active\nFirst,3,on\nSecond,7,\n"
	rows, err := ParseCSV(strings.NewReader(csv), widgetFields())
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if !rows[0].OK() {
		t.Fatalf("rows[0].Errors = %v, want none", rows[0].Errors)
	}
	if rows[0].Values["name"] != "First" || rows[0].Values["count"] != float64(3) || rows[0].Values["active"] != true {
		t.Fatalf("rows[0].Values = %+v, unexpected", rows[0].Values)
	}
	if rows[1].Values["active"] != false {
		t.Fatalf("rows[1].Values[active] = %v, want false (empty CSV cell)", rows[1].Values["active"])
	}
}

func TestParseCSV_UnknownColumnIgnored(t *testing.T) {
	csv := "name,mystery_column\nFirst,whatever\n"
	rows, err := ParseCSV(strings.NewReader(csv), widgetFields())
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if !rows[0].OK() {
		t.Fatalf("rows[0].Errors = %v, want none — an unknown column should be ignored, not error", rows[0].Errors)
	}
	if _, present := rows[0].Values["mystery_column"]; present {
		t.Fatal("rows[0].Values contains the unknown column, want it dropped")
	}
}

func TestParseCSV_MissingRequiredFieldErrors(t *testing.T) {
	csv := "name,count\n,5\n"
	rows, err := ParseCSV(strings.NewReader(csv), widgetFields())
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if rows[0].OK() {
		t.Fatal("rows[0] is OK, want a required-field error for an empty name cell")
	}
	if rows[0].Errors["name"] == "" {
		t.Fatalf("rows[0].Errors = %v, want a name error specifically", rows[0].Errors)
	}
}

func TestParseCSV_RaggedRowShorterThanHeader(t *testing.T) {
	csv := "name,count,active\nOnlyName\n"
	rows, err := ParseCSV(strings.NewReader(csv), widgetFields())
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if !rows[0].OK() {
		t.Fatalf("rows[0].Errors = %v, want none — missing trailing columns are just absent, not an error", rows[0].Errors)
	}
	if rows[0].Values["name"] != "OnlyName" {
		t.Fatalf("rows[0].Values[name] = %v, want %q", rows[0].Values["name"], "OnlyName")
	}
}

func TestParseCSV_HeaderOnlyYieldsNoRows(t *testing.T) {
	rows, err := ParseCSV(strings.NewReader("name,count,active\n"), widgetFields())
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("got %d rows, want 0 for a header-only file", len(rows))
	}
}

func TestParseCSV_EmptyFileYieldsNoRows(t *testing.T) {
	rows, err := ParseCSV(strings.NewReader(""), widgetFields())
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("got %d rows, want 0 for an empty file", len(rows))
	}
}

func TestParseCSV_RowIndexIs1Based(t *testing.T) {
	csv := "name\nFirst\nSecond\nThird\n"
	rows, err := ParseCSV(strings.NewReader(csv), widgetFields())
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	for i, want := range []int{1, 2, 3} {
		if rows[i].Index != want {
			t.Fatalf("rows[%d].Index = %d, want %d", i, rows[i].Index, want)
		}
	}
}

func TestParseCSV_BooleanTrueValues(t *testing.T) {
	for _, val := range []string{"true", "TRUE", "1", "yes", "Y", "on"} {
		csv := "name,active\nX," + val + "\n"
		rows, err := ParseCSV(strings.NewReader(csv), widgetFields())
		if err != nil {
			t.Fatalf("ParseCSV(%q): %v", val, err)
		}
		if rows[0].Values["active"] != true {
			t.Fatalf("value %q: active = %v, want true", val, rows[0].Values["active"])
		}
	}
}

func TestParseCSV_BooleanFalseValues(t *testing.T) {
	for _, val := range []string{"false", "FALSE", "0", "no", "N", "off", ""} {
		csv := "name,active\nX," + val + "\n"
		rows, err := ParseCSV(strings.NewReader(csv), widgetFields())
		if err != nil {
			t.Fatalf("ParseCSV(%q): %v", val, err)
		}
		if rows[0].Values["active"] != false {
			t.Fatalf("value %q: active = %v, want false", val, rows[0].Values["active"])
		}
	}
}

func TestParseCSV_BooleanUnrecognisedValueErrors(t *testing.T) {
	csv := "name,active\nX,maybe\n"
	rows, err := ParseCSV(strings.NewReader(csv), widgetFields())
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if rows[0].OK() {
		t.Fatal("rows[0] is OK, want an error for an unrecognised boolean value")
	}
	if rows[0].Errors["active"] == "" {
		t.Fatalf("errors = %v, want an active-field error", rows[0].Errors)
	}
}

func TestParseCSV_BooleanColumnAbsentFromHeaderOmitsField(t *testing.T) {
	// Distinct from an empty cell: the column isn't in the file at all.
	csv := "name\nX\n"
	rows, err := ParseCSV(strings.NewReader(csv), widgetFields())
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if _, present := rows[0].Values["active"]; present {
		t.Fatalf("Values[active] = %v, want omitted when the column isn't in the header at all", rows[0].Values["active"])
	}
}

func TestParseJSON_ValidRows(t *testing.T) {
	body := `[{"name":"First","count":3,"active":true},{"name":"Second","count":7,"active":false}]`
	rows, err := ParseJSON(strings.NewReader(body), widgetFields())
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if !rows[0].OK() || rows[0].Values["name"] != "First" || rows[0].Values["count"] != float64(3) {
		t.Fatalf("rows[0] = %+v, unexpected", rows[0])
	}
}

func TestParseJSON_MissingRequiredFieldErrors(t *testing.T) {
	body := `[{"count":5}]`
	rows, err := ParseJSON(strings.NewReader(body), widgetFields())
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	if rows[0].OK() {
		t.Fatal("rows[0] is OK, want a required-field error when name is absent entirely")
	}
}

func TestParseJSON_NullRequiredFieldErrors(t *testing.T) {
	body := `[{"name":null,"count":5}]`
	rows, err := ParseJSON(strings.NewReader(body), widgetFields())
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	if rows[0].OK() {
		t.Fatal("rows[0] is OK, want a required-field error when name is explicitly null")
	}
}

func TestParseJSON_MalformedJSONErrors(t *testing.T) {
	_, err := ParseJSON(strings.NewReader("{not valid json"), widgetFields())
	if err == nil {
		t.Fatal("ParseJSON: want an error for malformed JSON")
	}
}

func TestParseJSON_EmptyArrayYieldsNoRows(t *testing.T) {
	rows, err := ParseJSON(strings.NewReader("[]"), widgetFields())
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("got %d rows, want 0", len(rows))
	}
}

func TestParseJSON_RowIndexIs1Based(t *testing.T) {
	body := `[{"name":"A"},{"name":"B"}]`
	rows, err := ParseJSON(strings.NewReader(body), widgetFields())
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	if rows[0].Index != 1 || rows[1].Index != 2 {
		t.Fatalf("indices = %d, %d, want 1, 2", rows[0].Index, rows[1].Index)
	}
}
