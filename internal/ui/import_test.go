// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ha1tch/xoluman/internal/formengine"
	"github.com/ha1tch/xoluman/internal/importer"
)

// multipartUpload builds a multipart/form-data request body containing
// a "format" field and a "file" upload with the given content.
func multipartUpload(t *testing.T, format, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("format", format); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatalf("writing file content: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}
	return &buf, w.FormDataContentType()
}

// importerTestSession builds a session with one valid row and one
// invalid (missing-required-field) row — used to verify that confirm
// only ever attempts the valid one.
func importerTestSession(t *testing.T, h *EntitiesHandler, connName, entityType string) importer.Session {
	t.Helper()
	return importer.Session{
		ConnName:   connName,
		EntityType: entityType,
		Rows: []importer.Row{
			{Index: 1, Values: formengine.Values{"name": "Valid", "count": float64(1)}, Errors: nil},
			{Index: 2, Values: nil, Errors: formengine.Errors{"name": "This field is required."}},
		},
	}
}

func TestEntitiesHandler_ImportForm_RendersUploadFields(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/connections/test/entities/widgets/import", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.ImportForm(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `name="file"`) || !strings.Contains(body, `name="format"`) {
		t.Fatalf("body missing upload fields: %s", body)
	}
	if !strings.Contains(body, `enctype="multipart/form-data"`) {
		t.Fatalf("body missing multipart enctype: %s", body)
	}
}

func TestEntitiesHandler_ImportPreview_ValidCSV(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	body, contentType := multipartUpload(t, "csv", "widgets.csv", "name,count\nThird,10\nFourth,20\n")
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/widgets/import", body)
	req.Header.Set("Content-Type", contentType)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.ImportPreview(rec, req)

	respBody := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, respBody)
	}
	if !strings.Contains(respBody, "2 row(s) parsed") {
		t.Fatalf("body missing the parse summary: %s", respBody)
	}
	if !strings.Contains(respBody, "/import/") || !strings.Contains(respBody, "/confirm") {
		t.Fatalf("body missing the confirm form action: %s", respBody)
	}
}

func TestEntitiesHandler_ImportPreview_ValidJSON(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	body, contentType := multipartUpload(t, "json", "widgets.json", `[{"name":"Third","count":10}]`)
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/widgets/import", body)
	req.Header.Set("Content-Type", contentType)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.ImportPreview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "1 row(s) parsed") {
		t.Fatalf("body missing the parse summary: %s", rec.Body.String())
	}
}

func TestEntitiesHandler_ImportPreview_RowWithErrorShown(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	body, contentType := multipartUpload(t, "csv", "widgets.csv", "name,count\n,10\n")
	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/widgets/import", body)
	req.Header.Set("Content-Type", contentType)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.ImportPreview(rec, req)

	respBody := rec.Body.String()
	if !strings.Contains(respBody, "0 ready to import, 1 with errors") {
		t.Fatalf("body missing the correct ready/error split: %s", respBody)
	}
	if strings.Contains(respBody, "Import 0 row") {
		t.Fatalf("body offers a confirm button with 0 valid rows, want none: %s", respBody)
	}
}

func TestEntitiesHandler_ImportPreview_NoFileUploaded(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("format", "csv")
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/widgets/import", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	rec := httptest.NewRecorder()

	h.ImportPreview(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestEntitiesHandler_ImportConfirm_CreatesValidRowsOnly(t *testing.T) {
	var created []map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/widgets", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(widgetsSchemaJSON))
	})
	mux.HandleFunc("POST /api/v1/widgets", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		created = append(created, body)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(len(created)), "message": "created"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := NewEntitiesHandler(store)

	sessionID, err := h.sessions.Put(importerTestSession(t, h, "test", "widgets"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/widgets/import/"+sessionID+"/confirm", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	req.SetPathValue("id", sessionID)
	rec := httptest.NewRecorder()

	h.ImportConfirm(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(created) != 1 {
		t.Fatalf("created %d entities, want exactly 1 (the OK row; the errored row must not be attempted)", len(created))
	}
	if created[0]["name"] != "Valid" {
		t.Fatalf("created[0] = %+v, want the valid row's data", created[0])
	}
	if !strings.Contains(rec.Body.String(), "1 of 1 attempted row(s) created successfully") {
		t.Fatalf("body missing the correct result summary: %s", rec.Body.String())
	}
}

func TestEntitiesHandler_ImportConfirm_SessionIsSingleUse(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	sessionID, err := h.sessions.Put(importerTestSession(t, h, "test", "widgets"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	req1 := httptest.NewRequest(http.MethodPost, "/connections/test/entities/widgets/import/"+sessionID+"/confirm", nil)
	req1.SetPathValue("name", "test")
	req1.SetPathValue("type", "widgets")
	req1.SetPathValue("id", sessionID)
	rec1 := httptest.NewRecorder()
	h.ImportConfirm(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first confirm: status = %d, want %d", rec1.Code, http.StatusOK)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/connections/test/entities/widgets/import/"+sessionID+"/confirm", nil)
	req2.SetPathValue("name", "test")
	req2.SetPathValue("type", "widgets")
	req2.SetPathValue("id", sessionID)
	rec2 := httptest.NewRecorder()
	h.ImportConfirm(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("second confirm on the same session: status = %d, want %d (already consumed)", rec2.Code, http.StatusNotFound)
	}
}

func TestEntitiesHandler_ImportConfirm_UnknownSession(t *testing.T) {
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := NewEntitiesHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/connections/test/entities/widgets/import/does-not-exist/confirm", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("type", "widgets")
	req.SetPathValue("id", "does-not-exist")
	rec := httptest.NewRecorder()

	h.ImportConfirm(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
