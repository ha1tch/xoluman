// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package xoluext

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ha1tch/xolu/pkg/client"

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

func TestBuildClient_AuthModeAPIKey_SendsAPIKeyHeader(t *testing.T) {
	// "ApiKey ", not "Bearer " — xolu v0.27.0 (T-160) fixed a real bug
	// where the client sent Bearer-prefixed apikey auth, which the
	// server's own validateAPIKey never accepted; every AuthAPIKey
	// connection was silently unauthenticated on every request until
	// that fix. This test previously asserted the broken behavior
	// (it would have passed either way, since it only records what
	// header was sent rather than validating against a real,
	// credential-enforcing server — the same class of blind spot the
	// xolu team's own test suite had for this exact bug).
	server, gotAuth, _ := captureAuth(t)
	c := BuildClient(connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{BaseURL: server.URL, AuthMode: connstore.AuthAPIKey},
		Token:          "my-api-key",
	})
	_, _ = c.List(context.Background(), "widgets", nil)

	if *gotAuth != "ApiKey my-api-key" {
		t.Fatalf("Authorization header = %q, want %q", *gotAuth, "ApiKey my-api-key")
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

func TestBuildClient_GetEntitySchema_NoTenantPrefixEvenWithTenantSet(t *testing.T) {
	// Confirms xolu v0.27.0's actual fix, not just the absence of
	// xoluman's former workaround (BuildSchemaClient, removed once
	// this was verified — see docs/RESOLVED.md's T-23 entry). The
	// regular, tenant-configured client must now route a schema fetch
	// correctly on its own: GetEntitySchema hits a genuinely global
	// endpoint (`/schema/{entity}`), and xolu's client now knows that
	// via buildURLRoot (confirmed directly against
	// pkg/client/client.go's own doc comment on the fix) rather than
	// applying the tenant prefix indiscriminately the way it used to.
	server, _, gotPath := captureAuth(t)
	c := BuildClient(connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{BaseURL: server.URL, AuthMode: connstore.AuthNone, Tenant: "acme_crm"},
	})
	_, _ = c.GetEntitySchema(context.Background(), "companies")

	if strings.Contains(*gotPath, "/tenant/") {
		t.Fatalf("request path = %q, want no tenant prefix for a schema fetch regardless of the connection's configured tenant", *gotPath)
	}
	if *gotPath != "/api/v1/schema/companies" {
		t.Fatalf("request path = %q, want exactly %q", *gotPath, "/api/v1/schema/companies")
	}
}

// listAllMockServer simulates exactly the real-world situation that
// caused this bug: a server that ignores (or caps) the requested
// per_page and serves a smaller page anyway, but correctly reports
// page/total_pages/total_items in its own pagination envelope. This
// is the precise shape xolu's real server takes when Limit exceeds
// its documented maximum (docs/API_REFERENCE.md: "max 100") — ListAll
// must work correctly by following total_pages, not by trusting that
// its own requested Limit was honored.
func listAllMockServer(t *testing.T, totalItems, serverPageSize int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		start := (page - 1) * serverPageSize
		end := start + serverPageSize
		if end > totalItems {
			end = totalItems
		}
		var data []map[string]any
		for i := start; i < end; i++ {
			data = append(data, map[string]any{"id": i + 1, "name": "item"})
		}
		totalPages := (totalItems + serverPageSize - 1) / serverPageSize
		if totalPages < 1 {
			totalPages = 1
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": data,
			"pagination": map[string]any{
				"page": page, "per_page": serverPageSize,
				"total_items": totalItems, "total_pages": totalPages,
			},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestListWithEmbed_BuildsTenantScopedPathWithEmbedDepth(t *testing.T) {
	var gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":1,"name":"Alice","manager":{"id":7,"name":"Bob"}}],"pagination":{"page":1,"per_page":10,"total_items":1,"total_pages":1}}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	c := client.New(server.URL)
	conn := connstore.Connection{ConnectionMeta: connstore.ConnectionMeta{Tenant: "acme"}}

	result, err := ListWithEmbed(context.Background(), c, conn, "users", 1, &client.ListParams{Limit: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/tenant/acme/users?embed_depth=1&per_page=5" {
		t.Errorf("path = %q, want tenant-scoped path with embed_depth and per_page", gotPath)
	}
	if len(result.Entities) != 1 || result.Entities[0].ID != 1 {
		t.Fatalf("entities = %+v, want one entity with id 1", result.Entities)
	}
	manager, ok := result.Entities[0].Data["manager"].(map[string]any)
	if !ok || manager["name"] != "Bob" {
		t.Errorf("manager field = %+v, want the hydrated nested document", result.Entities[0].Data["manager"])
	}
	if result.TotalItems != 1 || result.TotalPages != 1 {
		t.Errorf("pagination = page=%d perPage=%d total=%d pages=%d, want the decoded envelope values",
			result.Page, result.PerPage, result.TotalItems, result.TotalPages)
	}
}

func TestListWithEmbed_NoTenantMeansUnscopedPath(t *testing.T) {
	var gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"page":1,"per_page":10,"total_items":0,"total_pages":1}}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	c := client.New(server.URL)
	conn := connstore.Connection{} // no tenant configured

	_, err := ListWithEmbed(context.Background(), c, conn, "users", 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/users" {
		t.Errorf("path = %q, want the un-tenant-scoped path", gotPath)
	}
}

func TestListWithEmbed_ZeroEmbedDepthOmitsTheParam(t *testing.T) {
	var gotQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"page":1,"per_page":10,"total_items":0,"total_pages":1}}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	c := client.New(server.URL)
	_, err := ListWithEmbed(context.Background(), c, connstore.Connection{}, "users", 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(gotQuery, "embed_depth") {
		t.Errorf("query = %q, want no embed_depth param when embedDepth is 0", gotQuery)
	}
}

func TestListWithEmbed_UpstreamErrorSurfaced(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"entity not found"}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	c := client.New(server.URL)
	_, err := ListWithEmbed(context.Background(), c, connstore.Connection{}, "nonexistent", 1, nil)
	if err == nil {
		t.Fatal("expected an error for a 404 response, got nil")
	}
	xoluErr, ok := err.(*client.Error)
	if !ok {
		t.Fatalf("error type = %T, want *client.Error", err)
	}
	if xoluErr.HTTPStatus != http.StatusNotFound {
		t.Errorf("HTTPStatus = %d, want 404", xoluErr.HTTPStatus)
	}
}

func TestListAll_FollowsPaginationAcrossMultiplePages(t *testing.T) {
	// 25 total items, server only ever serves 10 per page regardless of
	// what's requested — exactly the real per_page-exceeds-maximum
	// scenario, reproduced directly rather than assumed.
	server := listAllMockServer(t, 25, 10)
	c := client.New(server.URL)

	entities, err := ListAll(context.Background(), c, "widgets")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 25 {
		t.Fatalf("got %d entities, want 25 — pagination did not fetch everything", len(entities))
	}
	// Confirm no duplicates and no gaps: every id 1..25 present exactly once.
	seen := make(map[int64]bool)
	for _, e := range entities {
		if seen[e.ID] {
			t.Errorf("id %d returned more than once", e.ID)
		}
		seen[e.ID] = true
	}
	for id := int64(1); id <= 25; id++ {
		if !seen[id] {
			t.Errorf("id %d missing from result", id)
		}
	}
}

func TestListAll_SinglePageWhenTotalFitsInOnePage(t *testing.T) {
	server := listAllMockServer(t, 5, 100)
	c := client.New(server.URL)

	entities, err := ListAll(context.Background(), c, "widgets")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 5 {
		t.Fatalf("got %d entities, want 5", len(entities))
	}
}

func TestListAll_EmptyResult(t *testing.T) {
	server := listAllMockServer(t, 0, 10)
	c := client.New(server.URL)

	entities, err := ListAll(context.Background(), c, "widgets")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Fatalf("got %d entities, want 0", len(entities))
	}
}

func TestListAll_ExactMultipleOfPageSize(t *testing.T) {
	// 20 items, 10 per page -- exactly 2 full pages, no partial third
	// page. A common off-by-one trap: confirm it doesn't loop forever
	// or make a spurious extra request.
	var requestCount int
	server := listAllMockServer(t, 20, 10)
	origHandler := server.Config.Handler
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		origHandler.ServeHTTP(w, r)
	})

	c := client.New(server.URL)
	entities, err := ListAll(context.Background(), c, "widgets")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 20 {
		t.Fatalf("got %d entities, want 20", len(entities))
	}
	if requestCount != 2 {
		t.Fatalf("made %d requests, want exactly 2 (no spurious extra page fetched)", requestCount)
	}
}

func TestListAll_PropagatesUpstreamError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := client.New(server.URL)

	_, err := ListAll(context.Background(), c, "widgets")
	if err == nil {
		t.Fatal("expected an error from a failing upstream call, got nil")
	}
}
