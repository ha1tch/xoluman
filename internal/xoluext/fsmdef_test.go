// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package xoluext

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/connstore"
)

func testConn(t *testing.T, serverURL string) connstore.Connection {
	t.Helper()
	return connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{
			Name: "test", BaseURL: serverURL, AuthMode: connstore.AuthNone,
		},
	}
}

func TestFsmDefAuthHeader_NoneMode(t *testing.T) {
	conn := connstore.Connection{ConnectionMeta: connstore.ConnectionMeta{AuthMode: connstore.AuthNone}, Token: "irrelevant"}
	if got := fsmDefAuthHeader(conn); got != "" {
		t.Fatalf("fsmDefAuthHeader(none) = %q, want empty", got)
	}
}

func TestFsmDefAuthHeader_APIKeyMode(t *testing.T) {
	conn := connstore.Connection{ConnectionMeta: connstore.ConnectionMeta{AuthMode: connstore.AuthAPIKey}, Token: "secret123"}
	if got := fsmDefAuthHeader(conn); got != "Bearer secret123" {
		t.Fatalf("fsmDefAuthHeader(apikey) = %q, want %q", got, "Bearer secret123")
	}
}

func TestFsmDefAuthHeader_EmptyTokenIsNoHeader(t *testing.T) {
	conn := connstore.Connection{ConnectionMeta: connstore.ConnectionMeta{AuthMode: connstore.AuthAPIKey}, Token: ""}
	if got := fsmDefAuthHeader(conn); got != "" {
		t.Fatalf("fsmDefAuthHeader(empty token) = %q, want empty", got)
	}
}

func TestFsmDefURL_NoTenant(t *testing.T) {
	conn := connstore.Connection{ConnectionMeta: connstore.ConnectionMeta{BaseURL: "http://x", Tenant: ""}}
	got := fsmDefURL(conn, "/fsm/def")
	want := "http://x/api/v2/fsm/def"
	if got != want {
		t.Fatalf("fsmDefURL = %q, want %q", got, want)
	}
}

func TestFsmDefURL_WithTenant(t *testing.T) {
	conn := connstore.Connection{ConnectionMeta: connstore.ConnectionMeta{BaseURL: "http://x", Tenant: "acme"}}
	got := fsmDefURL(conn, "/fsm/def")
	want := "http://x/api/v2/tenant/acme/fsm/def"
	if got != want {
		t.Fatalf("fsmDefURL = %q, want %q", got, want)
	}
}

func TestCreateMachineDef_Success(t *testing.T) {
	var gotBody map[string]any
	var gotAuth, gotMethod, gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/fsm/def", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 7, "name": "door_lock", "created_at": "2026-08-03T00:00:00Z",
			"analysis": map[string]any{"reachable": true},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	conn := testConn(t, server.URL)
	conn.AuthMode = connstore.AuthAPIKey
	conn.Token = "tok123"

	spec := xclient.MachineSpec{Name: "door_lock", Initial: "locked", States: map[string]xclient.StateDef{"locked": {}}}
	result, err := CreateMachineDef(context.Background(), conn, spec)
	if err != nil {
		t.Fatalf("CreateMachineDef: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/api/v2/fsm/def" {
		t.Fatalf("request = %s %s, want POST /api/v2/fsm/def", gotMethod, gotPath)
	}
	if gotAuth != "Bearer tok123" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer tok123")
	}
	if gotBody["name"] != "door_lock" {
		t.Fatalf("request body name = %v, want door_lock", gotBody["name"])
	}
	if result.ID != 7 || result.Name != "door_lock" {
		t.Fatalf("result = %+v, unexpected", result)
	}
}

func TestCreateMachineDef_ValidationErrorDecoded(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/fsm/def", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": "XOLU-FSM004", "message": "unreachable state: locked"},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	_, err := CreateMachineDef(context.Background(), testConn(t, server.URL), xclient.MachineSpec{Name: "bad"})
	if err == nil {
		t.Fatal("want an error for a 422 response")
	}
	xoluErr, ok := err.(*xclient.Error)
	if !ok {
		t.Fatalf("err type = %T, want *xclient.Error", err)
	}
	if xoluErr.Code != "XOLU-FSM004" || xoluErr.HTTPStatus != 422 {
		t.Fatalf("xoluErr = %+v, unexpected", xoluErr)
	}
}

func TestCreateMachineDef_UnstructuredErrorFallsBackToRawBody(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/fsm/def", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error, not JSON"))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	_, err := CreateMachineDef(context.Background(), testConn(t, server.URL), xclient.MachineSpec{Name: "x"})
	if err == nil {
		t.Fatal("want an error for a 500 response")
	}
	xoluErr, ok := err.(*xclient.Error)
	if !ok {
		t.Fatalf("err type = %T, want *xclient.Error", err)
	}
	if !strings.Contains(xoluErr.Message, "internal server error") {
		t.Fatalf("xoluErr.Message = %q, want it to fall back to the raw body", xoluErr.Message)
	}
}

func TestReplaceMachineDef_Success(t *testing.T) {
	var gotMethod, gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v2/fsm/def/{id}", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 7, "name": "door_lock_v2", "analysis": map[string]any{}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	result, err := ReplaceMachineDef(context.Background(), testConn(t, server.URL), 7, xclient.MachineSpec{Name: "door_lock_v2"})
	if err != nil {
		t.Fatalf("ReplaceMachineDef: %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v2/fsm/def/7" {
		t.Fatalf("request = %s %s, want PUT /api/v2/fsm/def/7", gotMethod, gotPath)
	}
	if result.Name != "door_lock_v2" {
		t.Fatalf("result.Name = %q, unexpected", result.Name)
	}
}

func TestReplaceMachineDef_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v2/fsm/def/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-FSM001", "message": "not found"}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	_, err := ReplaceMachineDef(context.Background(), testConn(t, server.URL), 999, xclient.MachineSpec{})
	if err == nil {
		t.Fatal("want an error for a 404")
	}
	xoluErr := err.(*xclient.Error)
	if xoluErr.HTTPStatus != 404 {
		t.Fatalf("HTTPStatus = %d, want 404", xoluErr.HTTPStatus)
	}
}

func TestDeleteMachineDef_Success(t *testing.T) {
	var gotMethod string
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v2/fsm/def/{id}", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	err := DeleteMachineDef(context.Background(), testConn(t, server.URL), 7)
	if err != nil {
		t.Fatalf("DeleteMachineDef: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("method = %s, want DELETE", gotMethod)
	}
}

func TestDeleteMachineDef_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v2/fsm/def/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-FSM001", "message": "not found"}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	err := DeleteMachineDef(context.Background(), testConn(t, server.URL), 999)
	if err == nil {
		t.Fatal("want an error for a 404")
	}
}

func TestValidateMachineDef_ValidSpec(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/fsm/def/validate", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // always 200, even for invalid specs
		_ = json.NewEncoder(w).Encode(map[string]any{"valid": true, "analysis": map[string]any{"ok": true}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	result, err := ValidateMachineDef(context.Background(), testConn(t, server.URL), xclient.MachineSpec{Name: "good"})
	if err != nil {
		t.Fatalf("ValidateMachineDef: %v", err)
	}
	if !result.Valid {
		t.Fatal("result.Valid = false, want true")
	}
	if len(result.Errors) != 0 {
		t.Fatalf("result.Errors = %v, want none for a valid spec", result.Errors)
	}
}

func TestValidateMachineDef_InvalidSpecIsNotAGoError(t *testing.T) {
	// The endpoint always returns 200 — an invalid spec is a normal,
	// successful response with Valid: false, not a transport error.
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/fsm/def/validate", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"valid":  false,
			"errors": []map[string]string{{"code": "XOLU-FSM006", "message": "unreachable state"}},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	result, err := ValidateMachineDef(context.Background(), testConn(t, server.URL), xclient.MachineSpec{Name: "bad"})
	if err != nil {
		t.Fatalf("ValidateMachineDef returned a Go error for an invalid-but-well-formed response: %v", err)
	}
	if result.Valid {
		t.Fatal("result.Valid = true, want false")
	}
	if len(result.Errors) != 1 || result.Errors[0].Code != "XOLU-FSM006" {
		t.Fatalf("result.Errors = %v, unexpected", result.Errors)
	}
}

func TestFsmDefMethods_TenantScoped(t *testing.T) {
	var gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/tenant/{tenant}/fsm/def", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "name": "x"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	conn := testConn(t, server.URL)
	conn.Tenant = "acme"

	if _, err := CreateMachineDef(context.Background(), conn, xclient.MachineSpec{Name: "x"}); err != nil {
		t.Fatalf("CreateMachineDef: %v", err)
	}
	if gotPath != "/api/v2/tenant/acme/fsm/def" {
		t.Fatalf("request path = %q, want the tenant-scoped path", gotPath)
	}
}
