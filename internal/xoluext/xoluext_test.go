// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package xoluext

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ha1tch/xoluman/internal/connstore"
)

// captureAuth starts a test server that records the Authorization header
// and request path of the first request it receives, then returns a
// minimal valid response so the client's decode step doesn't error out
// before the request is even sent.
func captureAuth(t *testing.T) (server *httptest.Server, gotAuth *string, gotPath *string) {
	t.Helper()
	gotAuth = new(string)
	gotPath = new(string)
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotAuth = r.Header.Get("Authorization")
		*gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"entities":[],"total":0}`))
	}))
	t.Cleanup(server.Close)
	return server, gotAuth, gotPath
}

func TestBuildClient_AuthModeNone_NoAuthorizationHeader(t *testing.T) {
	server, gotAuth, _ := captureAuth(t)
	c := BuildClient(connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{BaseURL: server.URL, AuthMode: connstore.AuthNone},
	})
	_, _ = c.List(context.Background(), "widgets", nil)

	if *gotAuth != "" {
		t.Fatalf("Authorization header = %q, want empty for AuthNone", *gotAuth)
	}
}

func TestBuildClient_AuthModeAPIKey_SendsBearerToken(t *testing.T) {
	server, gotAuth, _ := captureAuth(t)
	c := BuildClient(connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{BaseURL: server.URL, AuthMode: connstore.AuthAPIKey},
		Token:          "my-api-key",
	})
	_, _ = c.List(context.Background(), "widgets", nil)

	if *gotAuth != "Bearer my-api-key" {
		t.Fatalf("Authorization header = %q, want %q", *gotAuth, "Bearer my-api-key")
	}
}

func TestBuildClient_AuthModeBearer_SendsBearerToken(t *testing.T) {
	server, gotAuth, _ := captureAuth(t)
	c := BuildClient(connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{BaseURL: server.URL, AuthMode: connstore.AuthBearer},
		Token:          "my-bearer-token",
	})
	_, _ = c.List(context.Background(), "widgets", nil)

	if *gotAuth != "Bearer my-bearer-token" {
		t.Fatalf("Authorization header = %q, want %q", *gotAuth, "Bearer my-bearer-token")
	}
}

func TestBuildClient_AuthModeJWT_SendsBearerToken(t *testing.T) {
	server, gotAuth, _ := captureAuth(t)
	c := BuildClient(connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{BaseURL: server.URL, AuthMode: connstore.AuthJWT},
		Token:          "my.jwt.token",
	})
	_, _ = c.List(context.Background(), "widgets", nil)

	if *gotAuth != "Bearer my.jwt.token" {
		t.Fatalf("Authorization header = %q, want %q", *gotAuth, "Bearer my.jwt.token")
	}
}

func TestBuildClient_Tenant_PrefixesRequestPath(t *testing.T) {
	server, _, gotPath := captureAuth(t)
	c := BuildClient(connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{BaseURL: server.URL, AuthMode: connstore.AuthNone, Tenant: "acme"},
	})
	_, _ = c.List(context.Background(), "widgets", nil)

	if !strings.Contains(*gotPath, "/tenant/acme/") {
		t.Fatalf("request path = %q, want it to contain %q", *gotPath, "/tenant/acme/")
	}
}

func TestBuildClient_NoTenant_NoTenantPrefix(t *testing.T) {
	server, _, gotPath := captureAuth(t)
	c := BuildClient(connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{BaseURL: server.URL, AuthMode: connstore.AuthNone},
	})
	_, _ = c.List(context.Background(), "widgets", nil)

	if strings.Contains(*gotPath, "/tenant/") {
		t.Fatalf("request path = %q, want no tenant prefix when Tenant is unset", *gotPath)
	}
}

func TestBuildSchemaClient_NeverAppliesTenantPrefix_EvenWhenTenantIsSet(t *testing.T) {
	// The whole point of this function's existence: real bug, found
	// via a real end-to-end report and reproduced exactly against a
	// live server (examples/crm), not hypothetical. xolu's
	// GetEntitySchema/DefineEntitySchema hit a genuinely global
	// endpoint (`/schema/{entity}`, registered only once on the
	// server, never duplicated under the tenant router) but
	// Client.do() applies the tenant prefix to every request
	// regardless once a tenant is configured — landing the entity
	// type name in xolu's entity-by-id route's numeric {id} slot and
	// failing with XOLU-ST004 ("Invalid ID"). BuildSchemaClient exists
	// specifically to never carry a tenant at all, closing this off
	// entirely rather than depending on call sites remembering not to
	// use the regular client for a schema fetch.
	server, _, gotPath := captureAuth(t)
	c := BuildSchemaClient(connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{BaseURL: server.URL, AuthMode: connstore.AuthNone, Tenant: "acme_crm"},
	})
	_, _ = c.GetEntitySchema(context.Background(), "companies")

	if strings.Contains(*gotPath, "/tenant/") {
		t.Fatalf("request path = %q, want no tenant prefix regardless of the connection's configured tenant", *gotPath)
	}
	if *gotPath != "/api/v1/schema/companies" {
		t.Fatalf("request path = %q, want exactly %q", *gotPath, "/api/v1/schema/companies")
	}
}

func TestBuildSchemaClient_StillAppliesAuth(t *testing.T) {
	// The workaround is specifically and only about the tenant prefix
	// — auth still needs to apply normally, or a schema fetch against
	// an authenticated instance would fail for an unrelated reason.
	server, gotAuth, _ := captureAuth(t)
	c := BuildSchemaClient(connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{BaseURL: server.URL, AuthMode: connstore.AuthAPIKey, Tenant: "acme_crm"},
		Token:          "secret-key",
	})
	_, _ = c.GetEntitySchema(context.Background(), "companies")

	if *gotAuth != "Bearer secret-key" {
		t.Fatalf("Authorization = %q, want %q", *gotAuth, "Bearer secret-key")
	}
}
