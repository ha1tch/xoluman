// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package xoluext bridges xoluman's own types onto xolu's Go client
// (github.com/ha1tch/xolu/pkg/client). BuildClient is the one thing in
// here today; anything that needs xolu's client extended beyond what it
// already offers (T-01/T-02/T-03 — blob, export, raw request methods)
// belongs upstream in xolu itself, not bolted on here.
package xoluext

import (
	"github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/connstore"
)

// BuildClient constructs a *client.Client configured from a stored
// Connection: base URL, auth mode/token, and default tenant if set.
func BuildClient(conn connstore.Connection) *client.Client {
	return client.New(conn.BaseURL, append(authOptions(conn), tenantOption(conn)...)...)
}

// BuildSchemaClient constructs a *client.Client identical to
// BuildClient except with no tenant configured — deliberately, not an
// oversight. xolu's schema endpoints (GetEntitySchema/
// DefineEntitySchema, both hitting `/schema/{entity}`) are global on
// the server, registered only once outside the tenant router group
// (confirmed directly against pkg/server/server.go's own route
// registration — no tenant-scoped duplicate exists for `/schema/...`,
// unlike `/entities`, schema-suggestion, and both promote endpoints,
// which genuinely are registered under both). But `Client.do()`
// applies the tenant path prefix to *every* request whenever
// `WithTenant` was set on the client, with no per-endpoint awareness
// of which ones are actually tenant-scoped. The result, confirmed
// directly against a real tenant-scoped server: a client built with a
// tenant configured requests `/api/v1/tenant/{tenant}/schema/{entity}`
// for a schema fetch — a path that doesn't exist as such, which xolu's
// router matches against its entity-by-id pattern instead, landing the
// entity type name (e.g. "companies") in the numeric {id} slot and
// failing with XOLU-ST004 ("Invalid ID"). This reproduced exactly,
// byte for byte, against the examples/crm demo — the report that
// actually pinned it down, after several sessions of not being able
// to reproduce it with simple, tenant-less test data.
//
// Filed as its own request to the xolu team (see docs/xolu-requests-
// tenant-schema.md) since the correct fix is on the client's own side
// — either buildURL should know which endpoints are tenant-scoped, or
// GetEntitySchema/DefineEntitySchema should build their own tenant-
// less URL directly. This is xoluman's own workaround in the
// meantime: every GetEntitySchema/DefineEntitySchema call site uses a
// client built with this function, not the connection's regular
// (correctly tenant-scoped) one used for actual entity data.
func BuildSchemaClient(conn connstore.Connection) *client.Client {
	return client.New(conn.BaseURL, authOptions(conn)...)
}

func authOptions(conn connstore.Connection) []client.ClientOption {
	var opts []client.ClientOption
	switch conn.AuthMode {
	case connstore.AuthAPIKey:
		opts = append(opts, client.WithAPIKey(conn.Token))
	case connstore.AuthBearer:
		opts = append(opts, client.WithBearerToken(conn.Token))
	case connstore.AuthJWT:
		opts = append(opts, client.WithJWT(conn.Token))
	case connstore.AuthNone:
		// No auth option needed — client.New defaults to no Authorization header.
	}
	return opts
}

func tenantOption(conn connstore.Connection) []client.ClientOption {
	if conn.Tenant == "" {
		return nil
	}
	return []client.ClientOption{client.WithTenant(conn.Tenant)}
}
