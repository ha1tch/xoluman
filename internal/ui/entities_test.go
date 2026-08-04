// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/connstore"
)

// widgetsSchema is the JSON Schema fakeXoluWithWidgets serves for the
// "widgets" entity type — one required string field, one optional
// integer field, matching the shapes formengine already has dedicated
// unit tests for individually.
const widgetsSchemaJSON = `{
  "type": "object",
  "properties": {
    "name": {"type": "string"},
    "count": {"type": "integer"}
  },
  "required": ["name"]
}`

// fakeXoluWithWidgets starts a fake xolu instance serving one entity
// type ("widgets") with two seeded rows, backing every endpoint the
// entity browser actually calls. store, when non-nil, receives calls
// the handlers make (create/update/delete) so tests can assert on them.
func fakeXoluWithWidgets(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/schemas", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schemas": []map[string]string{{"name": "widgets"}},
			"count":   1,
		})
	})

	mux.HandleFunc("GET /api/v1/entities", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entities": []map[string]any{{"entity_type": "widgets", "count": float64(2), "has_schema": true, "adapted": true}},
			"count":    1,
		})
	})

	mux.HandleFunc("GET /api/v1/schema/widgets", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(widgetsSchemaJSON))
	})

	mux.HandleFunc("GET /api/v1/widgets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": float64(1), "name": "First widget", "count": float64(3)},
				{"id": float64(2), "name": "Second widget", "count": float64(7)},
			},
			"pagination": map[string]any{"page": 1, "per_page": 25, "total_items": 2, "total_pages": 1},
		})
	})

	mux.HandleFunc("GET /api/v1/widgets/1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(1), "name": "First widget", "count": float64(3)})
	})

	mux.HandleFunc("GET /api/v1/widgets/999", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-EN001", "message": "not found", "status": 404}})
	})

	mux.HandleFunc("POST /api/v1/widgets", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["name"] == nil || body["name"] == "" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-VL001", "message": "name is required", "status": 422}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(3), "message": "created"})
	})

	mux.HandleFunc("PUT /api/v1/widgets/1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "updated"})
	})

	mux.HandleFunc("DELETE /api/v1/widgets/1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"deleted": true})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// seedConnection saves a connection named "test" pointing at serverURL
// into a fresh store, returning both.
func seedConnection(t *testing.T, serverURL string) connstore.Store {
	t.Helper()
	store := connstore.NewFileBackend(t.TempDir())
	if _, err := store.Save(context.Background(), connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{Name: "test", BaseURL: serverURL, AuthMode: connstore.AuthNone},
	}); err != nil {
		t.Fatalf("seeding connection: %v", err)
	}
	return store
}

func TestEntitiesHandler_List_ShowsEntityTypes(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities", nil)
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "widgets") {
		t.Fatalf("body does not list the widgets entity type: %s", rec.Body.String())
	}
}

func TestEntitiesHandler_Show_PaginationBarWhenMultiplePages(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/widgets", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(widgetsSchemaJSON))
	})
	mux.HandleFunc("GET /api/v1/widgets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       []map[string]any{{"id": float64(1), "name": "Row"}},
			"pagination": map[string]any{"page": 2, "per_page": 25, "total_items": 60, "total_pages": 3},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/widgets?page=2", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.Show(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, body)
	}
	if !strings.Contains(body, `page=1`) || !strings.Contains(body, `page=3`) {
		t.Fatalf("body missing pagination links: %s", body)
	}
}

func TestEntitiesHandler_Show_UpstreamErrorSurfaces(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/widgets", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/widgets", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.Show(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestPreviewFields_ExcludesObjectAndArray(t *testing.T) {
	fields := []xclient.FieldDef{
		{Name: "title", Type: "string"},
		{Name: "metadata", Type: "object"},
		{Name: "tags", Type: "array"},
		{Name: "count", Type: "integer"},
	}
	got := previewFields(fields)

	if len(got) != 2 {
		t.Fatalf("previewFields = %v, want exactly title and count (object/array excluded)", got)
	}
	for _, f := range got {
		if f.Type == "object" || f.Type == "array" {
			t.Fatalf("previewFields included a %s field, want none", f.Type)
		}
	}
}

func TestPreviewFields_CapsAtMaxPreviewColumns(t *testing.T) {
	fields := make([]xclient.FieldDef, 0, 10)
	for i := 0; i < 10; i++ {
		fields = append(fields, xclient.FieldDef{Name: fmt.Sprintf("f%d", i), Type: "string"})
	}
	got := previewFields(fields)

	if len(got) != maxPreviewColumns {
		t.Fatalf("previewFields returned %d fields, want %d", len(got), maxPreviewColumns)
	}
}

func TestPreviewValue(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, "—"},
		{"short string", "hello", "hello"},
		{"true", true, "true"},
		{"false", false, "false"},
		{"whole float", float64(42), "42"},
		{"fractional float", 3.5, "3.5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := previewValue(c.in); got != c.want {
				t.Fatalf("previewValue(%#v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestPreviewValue_TruncatesLongStrings(t *testing.T) {
	long := strings.Repeat("x", previewMaxStringLen+20)
	got := previewValue(long)

	if runeCount := len([]rune(got)); runeCount != previewMaxStringLen {
		t.Fatalf("previewValue truncated rune count = %d, want %d", runeCount, previewMaxStringLen)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("previewValue(long) = %q, want it to end with an ellipsis", got)
	}
}

func TestEntitiesHandler_GridView_RendersColumnsAndComponent(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/widgets/grid", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.GridView(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<xolu-grid-editor") {
		t.Fatalf("body missing the grid editor custom element: %s", body)
	}
	if !strings.Contains(body, `data-url="/connections/test/entities/widgets/grid-data"`) {
		t.Fatalf("body missing the correct data-url attribute: %s", body)
	}
	if !strings.Contains(body, `id="grid-columns"`) {
		t.Fatalf("body missing the grid-columns script tag: %s", body)
	}
	if !strings.Contains(body, `"field":"name"`) {
		t.Fatalf("body's column JSON missing the name field: %s", body)
	}
	if !strings.Contains(body, `/static/vendor/tabulator@6.5.2.min.js`) {
		t.Fatalf("body missing the Tabulator script: %s", body)
	}
	if !strings.Contains(body, `"imports":{"lit"`) {
		t.Fatalf("body missing the Lit import map: %s", body)
	}
}

func TestEntitiesHandler_GridView_UnknownConnection(t *testing.T) {
	h := NewEntitiesHandler(newTestStore(t))
	req := httptest.NewRequest(http.MethodGet, "/connections/nope/entities/widgets/grid", nil)
	req.SetPathValue("name", "nope")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.GridView(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestBuildGridColumns_ExcludesObjectAndArray(t *testing.T) {
	fields := []xclient.FieldDef{
		{Name: "title", Type: "string"},
		{Name: "metadata", Type: "object"},
		{Name: "tags", Type: "array"},
	}
	cols := buildGridColumns(fields)

	// id + title only — metadata/tags excluded
	if len(cols) != 2 {
		t.Fatalf("got %d columns, want 2 (id, title): %+v", len(cols), cols)
	}
}

func TestBuildGridColumns_IDColumnNotEditable(t *testing.T) {
	cols := buildGridColumns(nil)
	if len(cols) != 1 || cols[0].Field != "id" {
		t.Fatalf("cols = %+v, want just the id column for no fields", cols)
	}
	if cols[0].Editor != false {
		t.Fatalf("id column Editor = %v, want false (not editable)", cols[0].Editor)
	}
}

func TestBuildGridColumns_EditorPerType(t *testing.T) {
	fields := []xclient.FieldDef{
		{Name: "count", Type: "integer"},
		{Name: "active", Type: "boolean"},
		{Name: "price", Type: "string", Format: "decimal"},
		{Name: "author_id", Type: "integer", Format: "ref"},
		{Name: "title", Type: "string"},
	}
	cols := buildGridColumns(fields)
	byField := make(map[string]gridColumn)
	for _, c := range cols {
		byField[c.Field] = c
	}

	cases := map[string]any{
		"count":     "number",
		"active":    "tickCross",
		"price":     "input", // decimal stays text — no float64 round-trip
		"author_id": "input", // ref by raw ID
		"title":     "input",
	}
	for field, wantEditor := range cases {
		if byField[field].Editor != wantEditor {
			t.Fatalf("column %q Editor = %v, want %v", field, byField[field].Editor, wantEditor)
		}
	}
}

func TestEntitiesHandler_Show_SchemaLessTypeStillWorks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/gadgets", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-ST008", "message": "No schema found for gadgets"}})
	})
	mux.HandleFunc("GET /api/v1/gadgets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       []map[string]any{{"id": float64(1), "name": "Thing One"}},
			"pagination": map[string]any{"page": 1, "per_page": 25, "total_items": 1, "total_pages": 1},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/gadgets", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "gadgets")
	rec := httptest.NewRecorder()

	h.Show(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (a missing schema must not fail the whole page); body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Thing One") {
		t.Fatalf("body missing the real, schema-less entity data: %s", rec.Body.String())
	}
}

func TestEntitiesHandler_Show_GenuineUpstreamErrorStillSurfaces(t *testing.T) {
	// A real connectivity/server error on the schema fetch must not be
	// silently swallowed the same way a 404 is — only "no schema
	// registered" is a fine, expected condition to degrade from.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/widgets", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/widgets", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.Show(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d — a real server error must still surface, not be treated like a missing schema", rec.Code, http.StatusBadGateway)
	}
}

func TestEntitiesHandler_List_JumpByNameRedirects(t *testing.T) {
	h := NewEntitiesHandler(seedConnection(t, fakeXoluWithWidgets(t).URL))

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities?type=gadgets", nil)
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/connections/test/entities/gadgets" {
		t.Fatalf("Location = %q, unexpected", loc)
	}
}

func TestEntitiesHandler_List_NoEntitiesShowsJumpFormNotJustEmptyState(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/entities", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"entities": []map[string]any{}, "count": 0})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities", nil)
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.List(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `name="type"`) {
		t.Fatalf("body missing the jump-by-name input even with zero entities: %s", body)
	}
	if !strings.Contains(body, "No entity types have any data") {
		t.Fatalf("body missing the empty-state message: %s", body)
	}
}

func TestEntitiesHandler_List_ShowsCountAndSchemaStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/entities", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entities": []map[string]any{
				{"entity_type": "widgets", "count": float64(42), "has_schema": true},
				{"entity_type": "gadgets", "count": float64(3), "has_schema": false},
			},
			"count": 2,
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities", nil)
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.List(rec, req)

	body := rec.Body.String()
	for _, want := range []string{"widgets", "42", "has schema", "gadgets", "3", "no schema"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
}

func TestInferFieldsFromEntities_UnionAcrossRows(t *testing.T) {
	entities := []xclient.Entity{
		{ID: 1, Data: map[string]any{"id": float64(1), "name": "A", "count": float64(3)}},
		{ID: 2, Data: map[string]any{"id": float64(2), "name": "B", "active": true}},
	}
	fields := inferFieldsFromEntities(entities)

	byName := make(map[string]xclient.FieldDef)
	for _, f := range fields {
		byName[f.Name] = f
	}
	if _, ok := byName["id"]; ok {
		t.Fatal("inferred fields include \"id\", want it excluded (identity, not a data field)")
	}
	if byName["name"].Type != "string" || byName["count"].Type != "number" || byName["active"].Type != "boolean" {
		t.Fatalf("byName = %+v, unexpected types", byName)
	}
}

func TestIsNotFoundError_True(t *testing.T) {
	err := &xclient.Error{HTTPStatus: 404}
	if !isNotFoundError(err) {
		t.Fatal("isNotFoundError(404) = false, want true")
	}
}

func TestIsNotFoundError_FalseForOtherStatus(t *testing.T) {
	err := &xclient.Error{HTTPStatus: 500}
	if isNotFoundError(err) {
		t.Fatal("isNotFoundError(500) = true, want false")
	}
}

func TestIsNotFoundError_FalseForNonXoluError(t *testing.T) {
	if isNotFoundError(errors.New("some other error")) {
		t.Fatal("isNotFoundError on a plain error = true, want false")
	}
}

func TestEntitiesHandler_NewForm_SchemaLessInfersFromExistingRow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/gadgets", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-ST008", "message": "No schema found for gadgets"}})
	})
	mux.HandleFunc("GET /api/v1/gadgets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       []map[string]any{{"id": float64(1), "name": "Existing Thing", "count": float64(5)}},
			"pagination": map[string]any{"page": 1, "per_page": 1, "total_items": 1, "total_pages": 1},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/gadgets/new", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "gadgets")
	rec := httptest.NewRecorder()

	h.NewForm(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="name"`) || !strings.Contains(body, `name="count"`) {
		t.Fatalf("body missing fields inferred from the existing row: %s", body)
	}
}

func TestEntitiesHandler_NewForm_SchemaLessNoDataShowsClearMessage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/gadgets", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-ST008", "message": "No schema found"}})
	})
	mux.HandleFunc("GET /api/v1/gadgets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       []map[string]any{},
			"pagination": map[string]any{"page": 1, "per_page": 1, "total_items": 0, "total_pages": 0},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/gadgets/new", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "gadgets")
	rec := httptest.NewRecorder()

	h.NewForm(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (a clear message, not an error status)", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "no existing rows to infer fields from") {
		t.Fatalf("body missing the clear no-data message: %s", rec.Body.String())
	}
}

func TestEntitiesHandler_Create_SchemaLessInfersAndCreates(t *testing.T) {
	var created map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/gadgets", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-ST008", "message": "not found"}})
	})
	mux.HandleFunc("GET /api/v1/gadgets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       []map[string]any{{"id": float64(1), "name": "Existing", "count": float64(1)}},
			"pagination": map[string]any{"page": 1, "per_page": 1, "total_items": 1, "total_pages": 1},
		})
	})
	mux.HandleFunc("POST /api/v1/gadgets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&created)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(2), "message": "created"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	form := url.Values{"name": {"New Thing"}, "count": {"9"}}
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/gadgets", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "gadgets")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	if created["name"] != "New Thing" || created["count"] != float64(9) {
		t.Fatalf("created = %+v, want name and count correctly typed from inferred fields", created)
	}
}

func TestEntitiesHandler_EditForm_SchemaLessInfersFromTheEntityItself(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/gadgets", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-ST008", "message": "not found"}})
	})
	mux.HandleFunc("GET /api/v1/gadgets/1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(1), "name": "Thing One", "active": true})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/gadgets/1/edit", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "gadgets")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.EditForm(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `value="Thing One"`) {
		t.Fatalf("body missing the prepopulated name: %s", body)
	}
	if !strings.Contains(body, `type="checkbox"`) {
		t.Fatalf("body missing the inferred boolean field as a checkbox: %s", body)
	}
}

func TestEntitiesHandler_Update_SchemaLessInfersFromTheSpecificRow(t *testing.T) {
	var patched map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/gadgets", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-ST008", "message": "not found"}})
	})
	mux.HandleFunc("GET /api/v1/gadgets/1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(1), "name": "Old Name", "count": float64(1)})
	})
	mux.HandleFunc("PUT /api/v1/gadgets/1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&patched)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "updated"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	form := url.Values{"name": {"New Name"}, "count": {"7"}}
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/gadgets/1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "gadgets")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	if patched["name"] != "New Name" || patched["count"] != float64(7) {
		t.Fatalf("patched = %+v, want correctly-typed values from inferred fields", patched)
	}
}

func TestEntitiesHandler_Update_SchemaLessEntityGoneIsARealError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/gadgets", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-ST008", "message": "not found"}})
	})
	mux.HandleFunc("GET /api/v1/gadgets/1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-EN001", "message": "not found"}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	form := url.Values{"name": {"New Name"}}
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/gadgets/1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "gadgets")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d — a genuinely vanished row must surface as a real error, not silently degrade", rec.Code, http.StatusBadGateway)
	}
}

func TestEntitiesHandler_GenuineSchemaErrorStillSurfaces(t *testing.T) {
	// Distinguishing a real server error on the schema fetch from "no
	// schema registered" matters across all four handlers — spot-check
	// NewForm here, the same isNotFoundError gate all four share.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/gadgets", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/gadgets/new", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "gadgets")
	rec := httptest.NewRecorder()

	h.NewForm(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestEntitiesHandler_Show_RefFieldRendersAsLinkInPreview(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/posts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":       "object",
			"properties": map[string]any{"title": map[string]any{"type": "string"}, "author_id": map[string]any{"type": "integer", "format": "ref", "target": "users"}},
		})
	})
	mux.HandleFunc("GET /api/v1/posts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       []map[string]any{{"id": float64(1), "title": "Hello", "author_id": float64(7)}},
			"pagination": map[string]any{"page": 1, "per_page": 25, "total_items": 1, "total_pages": 1},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/posts", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "posts")
	rec := httptest.NewRecorder()

	h.Show(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `href="/connections/test/entities/users/7/edit"`) {
		t.Fatalf("body missing the ref link for author_id: %s", body)
	}
	if !strings.Contains(body, ">7<") {
		t.Fatalf("body missing the raw ID as link text (no label resolution in the list view — N+1 concern): %s", body)
	}
}

func TestEntitiesHandler_Show_SchemaLessTypeHasNoRefLinks(t *testing.T) {
	// Inference can't know which fields are references — must not
	// crash or fabricate a link for a schema-less type's integer field.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/gadgets", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-ST008", "message": "not found"}})
	})
	mux.HandleFunc("GET /api/v1/gadgets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       []map[string]any{{"id": float64(1), "owner_id": float64(3)}},
			"pagination": map[string]any{"page": 1, "per_page": 25, "total_items": 1, "total_pages": 1},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/gadgets", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "gadgets")
	rec := httptest.NewRecorder()

	h.Show(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "/entities//") {
		t.Fatalf("body contains a malformed ref link (empty target type): %s", rec.Body.String())
	}
}

func TestRefTargetsByField_NilSchema(t *testing.T) {
	if got := refTargetsByField(nil); got != nil {
		t.Fatalf("refTargetsByField(nil) = %v, want nil", got)
	}
}

func TestRefTargetsByField_SkipsEmptyTarget(t *testing.T) {
	schema := &xclient.EntitySchema{Refs: []xclient.RefFieldDef{{Name: "target_id", Target: ""}, {Name: "author_id", Target: "users"}}}
	got := refTargetsByField(schema)
	if len(got) != 1 || got["author_id"] != "users" {
		t.Fatalf("got = %v, want only author_id (polymorphic target_id with empty Target excluded)", got)
	}
}

func TestEntitiesHandler_List_UnknownConnection(t *testing.T) {
	h := NewEntitiesHandler(connstore.NewFileBackend(t.TempDir()))
	req := httptest.NewRequest(http.MethodGet, "/connections/nope/entities", nil)
	req.SetPathValue("name", "nope")
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestEntitiesHandler_Show_ListsEntities(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/widgets", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.Show(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{"First widget", "Second widget"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	if !strings.Contains(body, `href="/connections/test/entities/widgets/1/edit"`) {
		t.Fatalf("body missing edit link for entity 1: %s", body)
	}
}

func TestEntitiesHandler_NewForm_RendersSchemaFields(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/widgets/new", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.NewForm(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `name="name"`) || !strings.Contains(body, `name="count"`) {
		t.Fatalf("body missing expected form fields: %s", body)
	}
}

func TestEntitiesHandler_Create_Success(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	form := url.Values{"name": {"New widget"}, "count": {"5"}}
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/widgets", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/connections/test/entities/widgets" {
		t.Fatalf("Location = %q, unexpected", loc)
	}
}

func TestEntitiesHandler_Create_MissingRequiredFieldRedisplays(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	form := url.Values{"name": {""}, "count": {"5"}}
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/widgets", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if !strings.Contains(rec.Body.String(), "required") {
		t.Fatalf("body doesn't mention the required-field error: %s", rec.Body.String())
	}
}

func TestEntitiesHandler_EditForm_PrepopulatesValues(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/widgets/1/edit", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.EditForm(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `value="First widget"`) {
		t.Fatalf("body doesn't prepopulate the existing name: %s", body)
	}
	if !strings.Contains(body, `value="3"`) {
		t.Fatalf("body doesn't prepopulate the existing count: %s", body)
	}
}

func TestEntitiesHandler_EditForm_NotFoundEntity(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/widgets/999/edit", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	req.SetPathValue("id", "999")
	rec := httptest.NewRecorder()

	h.EditForm(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d (upstream error surfaced)", rec.Code, http.StatusBadGateway)
	}
}

func TestEntitiesHandler_Update_Success(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	form := url.Values{"name": {"Updated widget"}, "count": {"9"}}
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/widgets/1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
}

func TestEntitiesHandler_DeleteConfirm_RendersFragment(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/widgets/1/delete-confirm", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	req.SetPathValue("id", "1")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	h.DeleteConfirm(rec, req)

	if !strings.Contains(rec.Body.String(), "widgets #1") {
		t.Fatalf("body doesn't confirm deleting widgets #1: %s", rec.Body.String())
	}
}

func TestEntitiesHandler_Delete_Success(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/widgets/1/delete", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
}

func TestEntitiesHandler_Delete_InvalidID(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/widgets/not-a-number/delete", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	req.SetPathValue("id", "not-a-number")
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
