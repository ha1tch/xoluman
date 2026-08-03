// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package connstore manages xoluman's registry of named xolu instance
// connections — local and remote, each with its own base URL, auth mode,
// token, and optional default tenant.
//
// Two backends implement Store: the file backend (fileBackend, T-06)
// persists everything, including the token, to a single JSON file. The
// keyring backend (T-04) persists connection metadata to JSON but stores
// the token in the OS keyring, keyed by connection name. Which backend is
// active is a single xoluman-level setting chosen once at first run (see
// internal/config), not a per-connection choice.
//
// ConnectionMeta is deliberately the non-secret subset of Connection: the
// keyring backend's on-disk JSON contains only ConnectionMeta values, never
// a token. The file backend's on-disk JSON contains full Connection values.
// Callers of Store always work with Connection; the split only matters
// inside the two backend implementations.
package connstore

import (
	"context"
	"errors"
)

// AuthMode mirrors github.com/ha1tch/xolu/pkg/client's AuthMode values
// (AuthNone, AuthAPIKey, AuthBearer, AuthJWT) so a stored Connection maps
// directly onto a client.ClientOption without translation.
type AuthMode string

const (
	AuthNone   AuthMode = "none"
	AuthAPIKey AuthMode = "apikey"
	AuthBearer AuthMode = "bearertoken"
	AuthJWT    AuthMode = "jwt"
)

// Valid reports whether m is one of the four recognised auth modes.
func (m AuthMode) Valid() bool {
	switch m {
	case AuthNone, AuthAPIKey, AuthBearer, AuthJWT:
		return true
	default:
		return false
	}
}

// ConnectionMeta is the non-secret subset of Connection — everything the
// keyring backend is willing to write to disk in plain JSON.
type ConnectionMeta struct {
	Name     string   `json:"name"`
	BaseURL  string   `json:"base_url"`
	AuthMode AuthMode `json:"auth_mode"`
	Tenant   string   `json:"tenant,omitempty"`
}

// Connection is a single named xolu instance connection, including its
// secret. The file backend persists this whole struct to disk; the
// keyring backend never persists Token to disk — see connstore.go's
// package doc.
type Connection struct {
	ConnectionMeta
	Token string `json:"token,omitempty"`
}

// Sentinel errors returned by Store implementations.
var (
	// ErrNotFound is returned by Get and Delete when no connection with
	// the given name exists.
	ErrNotFound = errors.New("connstore: connection not found")

	// ErrInvalidName is returned when a connection name is empty. Names
	// are used as both JSON map keys and OS keyring usernames, so they
	// must be non-empty; no other restriction is imposed.
	ErrInvalidName = errors.New("connstore: connection name must not be empty")

	// ErrInvalidAuthMode is returned when a Connection's AuthMode is not
	// one of the four recognised values.
	ErrInvalidAuthMode = errors.New("connstore: invalid auth mode")
)

// Store is the interface both backends implement. Save upserts by name —
// there is no separate Create/Update distinction, matching the "created
// bool" convention already used by xolu's own client.Client.Save.
type Store interface {
	// List returns all stored connections, ordered by name.
	List(ctx context.Context) ([]Connection, error)

	// Get returns the named connection, or ErrNotFound.
	Get(ctx context.Context, name string) (Connection, error)

	// Save creates or updates the named connection. created is true when
	// no connection with this name existed before the call.
	Save(ctx context.Context, conn Connection) (created bool, err error)

	// Delete removes the named connection, or returns ErrNotFound.
	Delete(ctx context.Context, name string) error
}

// validate checks the fields Save requires regardless of backend: a
// non-empty name and a recognised auth mode. Backends call this before
// doing any I/O.
func validate(conn Connection) error {
	if conn.Name == "" {
		return ErrInvalidName
	}
	if !conn.AuthMode.Valid() {
		return ErrInvalidAuthMode
	}
	return nil
}
