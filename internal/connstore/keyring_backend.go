// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package connstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/zalando/go-keyring"
)

// keyringService is the service name every xoluman connection's token is
// stored under in the OS keyring, keyed by connection name as the
// keyring "user".
const keyringService = "xoluman"

// ErrTokenUnavailable is returned when a connection's metadata exists but
// its token is missing from the OS keyring — e.g. removed externally via
// the OS's own keyring UI, or the keyring service was reset. Distinct
// from ErrNotFound, which means the connection itself is unknown.
var ErrTokenUnavailable = errors.New("connstore: token not found in OS keyring for known connection")

// metaFileData is the on-disk shape of connections-meta.json — metadata
// only, never a token.
type metaFileData struct {
	Connections map[string]ConnectionMeta `json:"connections"`
}

// KeyringBackend is the Store implementation that persists connection
// metadata to plaintext JSON and tokens to the OS keyring. Requires an OS
// keyring service to be present at runtime — see T-04 and the dormant
// guard recorded in docs/KNOWN_ISSUES.md; there is no OS keyring service
// in the sandbox this was developed in, so behavioural correctness here
// is verified against keyring.MockInit()'s in-memory provider (see
// keyring_backend_test.go), and real-keyring round-tripping is a
// separate, build-tag-gated test for Horacio's local machine.
type KeyringBackend struct {
	path string
	mu   sync.Mutex
}

// NewKeyringBackend returns a KeyringBackend backed by
// <dir>/connections-meta.json plus the OS keyring.
func NewKeyringBackend(dir string) *KeyringBackend {
	return &KeyringBackend{path: filepath.Join(dir, "connections-meta.json")}
}

func (k *KeyringBackend) loadMeta() (metaFileData, error) {
	data, err := os.ReadFile(k.path)
	if errors.Is(err, os.ErrNotExist) {
		return metaFileData{Connections: map[string]ConnectionMeta{}}, nil
	}
	if err != nil {
		return metaFileData{}, err
	}
	var md metaFileData
	if err := json.Unmarshal(data, &md); err != nil {
		return metaFileData{}, err
	}
	if md.Connections == nil {
		md.Connections = map[string]ConnectionMeta{}
	}
	return md, nil
}

func (k *KeyringBackend) saveMeta(md metaFileData) error {
	data, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return err
	}
	tmp := k.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, k.path)
}

// hydrate looks up meta's token in the keyring and combines them into a
// full Connection. If the keyring has no entry for this name, Token is
// left empty and ok is false — the caller decides whether that's fatal
// (Get: yes, ErrTokenUnavailable; List: no, degrade and continue so one
// missing token doesn't blank out the whole list).
func hydrate(meta ConnectionMeta) (conn Connection, ok bool) {
	token, err := keyring.Get(keyringService, meta.Name)
	if err != nil {
		return Connection{ConnectionMeta: meta}, false
	}
	return Connection{ConnectionMeta: meta, Token: token}, true
}

func (k *KeyringBackend) List(ctx context.Context) ([]Connection, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	md, err := k.loadMeta()
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(md.Connections))
	for _, meta := range md.Connections {
		conn, _ := hydrate(meta) // missing token degrades to Token="", not fatal for List
		out = append(out, conn)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (k *KeyringBackend) Get(ctx context.Context, name string) (Connection, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	md, err := k.loadMeta()
	if err != nil {
		return Connection{}, err
	}
	meta, found := md.Connections[name]
	if !found {
		return Connection{}, ErrNotFound
	}
	conn, ok := hydrate(meta)
	if !ok {
		return Connection{}, ErrTokenUnavailable
	}
	return conn, nil
}

// Save writes the keyring entry before the metadata file, deliberately:
// if keyring.Set fails, nothing is persisted at all — no orphaned
// metadata describing a connection with no token. If the metadata write
// fails after keyring.Set succeeded, Save attempts a best-effort
// keyring.Delete to roll back rather than leaving a token stranded with
// no corresponding metadata; that rollback's own error is not surfaced
// (the original metadata-write error is what the caller needs to see).
func (k *KeyringBackend) Save(ctx context.Context, conn Connection) (bool, error) {
	if err := validate(conn); err != nil {
		return false, err
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	if err := keyring.Set(keyringService, conn.Name, conn.Token); err != nil {
		return false, fmt.Errorf("connstore: writing token to OS keyring: %w", err)
	}

	md, err := k.loadMeta()
	if err != nil {
		_ = keyring.Delete(keyringService, conn.Name) // best-effort rollback
		return false, err
	}
	_, existed := md.Connections[conn.Name]
	md.Connections[conn.Name] = conn.ConnectionMeta
	if err := k.saveMeta(md); err != nil {
		_ = keyring.Delete(keyringService, conn.Name) // best-effort rollback
		return false, err
	}
	return !existed, nil
}

// Delete removes metadata first — that is the source of truth for
// whether a connection exists — then attempts a best-effort keyring
// cleanup. A keyring.Delete failure after metadata is already gone does
// not fail the overall Delete: from the caller's perspective the
// connection is gone (List/Get will not see it), and an orphaned keyring
// secret is a smaller problem than a Delete that appears to fail when it
// didn't. keyring.ErrNotFound specifically is expected and ignored.
func (k *KeyringBackend) Delete(ctx context.Context, name string) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	md, err := k.loadMeta()
	if err != nil {
		return err
	}
	if _, ok := md.Connections[name]; !ok {
		return ErrNotFound
	}
	delete(md.Connections, name)
	if err := k.saveMeta(md); err != nil {
		return err
	}
	_ = keyring.Delete(keyringService, name) // best-effort; see doc comment
	return nil
}

// Compile-time assertion that KeyringBackend implements Store.
var _ Store = (*KeyringBackend)(nil)
