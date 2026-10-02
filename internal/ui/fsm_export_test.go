// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// realMachineDefBody is a genuinely valid MachineSpec, wrapped as the
// real MachineDef response shape GetMachineDef expects — reused by
// every export test below.
const realMachineDefBody = `{
	"id": 1,
	"created_at": "2026-01-01T00:00:00Z",
	"spec": {
		"name": "approval-flow",
		"initial": "pending",
		"determinism": "firstmatch",
		"states": {"pending": {}, "approved": {}},
		"transitions": [{"from": "pending", "to": "approved", "input": "approve"}]
	}
}`

func fakeXoluForFSMExport(t *testing.T, layoutDocs []map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v2/fsm/def/1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(realMachineDefBody))
	})
	mux.HandleFunc("GET /api/v1/xoluman_fsm_layout", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       layoutDocs,
			"pagination": map[string]any{"page": 1, "per_page": 100, "total_items": len(layoutDocs), "total_pages": 1},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestFSMHandler_ExportSVG_Succeeds(t *testing.T) {
	server := fakeXoluForFSMExport(t, nil)
	store := seedConnection(t, server.URL)
	h := &fsmHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/fsm/1/export/svg", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.ExportSVG(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("Content-Type = %q, want image/svg+xml", ct)
	}
	if !strings.Contains(rec.Body.String(), "<svg") {
		t.Errorf("body does not look like real SVG: %s", rec.Body.String()[:min(200, rec.Body.Len())])
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "approval-flow.svg") {
		t.Errorf("Content-Disposition = %q, want it to name the machine", rec.Header().Get("Content-Disposition"))
	}
}

func TestFSMHandler_ExportPNG_Succeeds(t *testing.T) {
	server := fakeXoluForFSMExport(t, nil)
	store := seedConnection(t, server.URL)
	h := &fsmHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/fsm/1/export/png", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.ExportPNG(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	// Real PNG magic bytes: 0x89 P N G \r \n 0x1A \n.
	body := rec.Body.Bytes()
	pngMagic := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if len(body) < len(pngMagic) || string(body[:len(pngMagic)]) != string(pngMagic) {
		t.Error("body does not start with real PNG magic bytes")
	}
}

func TestFSMHandler_ExportLaTeX_FailsWithoutSavedLayout(t *testing.T) {
	server := fakeXoluForFSMExport(t, nil) // no layout entities at all
	store := seedConnection(t, server.URL)
	h := &fsmHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/fsm/1/export/latex", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.ExportLaTeX(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (a clear, actionable error, not a panic or a fabricated layout)", rec.Code, http.StatusUnprocessableEntity)
	}
	if !strings.Contains(rec.Body.String(), "save") {
		t.Errorf("error body = %q, want it to actually tell the person what to do", rec.Body.String())
	}
}

func TestFSMHandler_ExportLaTeX_SucceedsWithSavedLayout(t *testing.T) {
	layoutDocs := []map[string]any{
		{
			"id":            1,
			"definition_id": float64(1),
			"layout": map[string]any{
				"version":       1,
				"canvasOffsetX": 0,
				"canvasOffsetY": 0,
				"states": map[string]any{
					"pending":  map[string]any{"x": 0, "y": 0},
					"approved": map[string]any{"x": 200, "y": 0},
				},
			},
		},
	}
	server := fakeXoluForFSMExport(t, layoutDocs)
	store := seedConnection(t, server.URL)
	h := &fsmHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/fsm/1/export/latex", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.ExportLaTeX(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-tex" {
		t.Errorf("Content-Type = %q, want application/x-tex", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{`\documentclass`, `\begin{tikzpicture}`, `\end{document}`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing expected LaTeX structural element %q", want)
		}
	}
}

func TestFSMHandler_Export_InvalidID(t *testing.T) {
	server := fakeXoluForFSMExport(t, nil)
	store := seedConnection(t, server.URL)
	h := &fsmHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/fsm/not-a-number/export/svg", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("id", "not-a-number")
	rec := httptest.NewRecorder()

	h.ExportSVG(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"approval-flow": "approval-flow",
		"My Machine":    "My_Machine",
		"a/b/../c":      "a_b____c",
		"":              "machine",
		"!!!":           "___",
	}
	for input, want := range cases {
		if got := sanitizeFilename(input); got != want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", input, got, want)
		}
	}
}
