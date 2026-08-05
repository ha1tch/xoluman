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
//
// A tenant-less variant of this (BuildSchemaClient) used to be
// needed as a workaround for a real xolu bug — GetEntitySchema/
// DefineEntitySchema being wrongly tenant-prefixed, filed as
// docs/xolu-requests-tenant-schema.md (T-23). Fixed directly in xolu
// v0.27.0 (buildURLRoot, confirmed against pkg/client/client.go's own
// doc comment on the fix) — the workaround was removed once that was
// verified, rather than kept around out of caution. See
// docs/RESOLVED.md's T-23 entry for the full history.
func BuildClient(conn connstore.Connection) *client.Client {
	return client.New(conn.BaseURL, append(authOptions(conn), tenantOption(conn)...)...)
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
