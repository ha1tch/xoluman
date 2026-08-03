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
