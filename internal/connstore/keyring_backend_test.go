// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package connstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestMain lives in testmain_mock_test.go / testmain_live_test.go, split
// by the keyring_live build tag, so the same tests below run against
// either go-keyring's in-memory mock (default) or a real OS keyring
// provider (-tags keyring_live) without duplicating any test logic. See
// the dormant-guard entry in docs/KNOWN_ISSUES.md for the invocation.

// cleanupKeyringEntry deletes the given connection's keyring entry when
// the test ends, regardless of provider (mock or real). Harmless under
// the mock; under -tags keyring_live this is what stops these tests from
// leaving real entries behind in the OS keyring under the "xoluman"
// service.
func cleanupKeyringEntry(t *testing.T, name string) {
	t.Helper()
	t.Cleanup(func() { _ = keyring.Delete(keyringService, name) })
}

func TestKeyringBackend_SaveCreatesThenUpdates(t *testing.T) {
	dir := t.TempDir()
	kb := NewKeyringBackend(dir)
	ctx := context.Background()
	cleanupKeyringEntry(t, "local")

	created, err := kb.Save(ctx, testConn("local"))
	if err != nil {
		t.Fatalf("Save (create): %v", err)
	}
	if !created {
		t.Fatal("Save (create): want created=true, got false")
	}

	updated := testConn("local")
	updated.Token = "rotated-token"
	created, err = kb.Save(ctx, updated)
	if err != nil {
		t.Fatalf("Save (update): %v", err)
	}
	if created {
		t.Fatal("Save (update): want created=false, got true")
	}

	got, err := kb.Get(ctx, "local")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Token != "rotated-token" {
		t.Fatalf("Get after update: Token = %q, want %q", got.Token, "rotated-token")
	}
}

func TestKeyringBackend_GetNotFound(t *testing.T) {
	kb := NewKeyringBackend(t.TempDir())
	_, err := kb.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(missing): err = %v, want ErrNotFound", err)
	}
}

func TestKeyringBackend_DeleteNotFound(t *testing.T) {
	kb := NewKeyringBackend(t.TempDir())
	err := kb.Delete(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(missing): err = %v, want ErrNotFound", err)
	}
}

func TestKeyringBackend_DeleteRemovesMetaAndToken(t *testing.T) {
	dir := t.TempDir()
	kb := NewKeyringBackend(dir)
	ctx := context.Background()
	cleanupKeyringEntry(t, "local") // no-op if Delete below already removed it; belt and braces

	if _, err := kb.Save(ctx, testConn("local")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := kb.Delete(ctx, "local"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := kb.Get(ctx, "local"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete: err = %v, want ErrNotFound", err)
	}
	// The keyring entry itself should be gone too, not just the metadata.
	if _, err := keyring.Get(keyringService, "local"); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("keyring.Get after Delete: err = %v, want keyring.ErrNotFound", err)
	}
}

func TestKeyringBackend_ListSortedByName(t *testing.T) {
	dir := t.TempDir()
	kb := NewKeyringBackend(dir)
	ctx := context.Background()

	for _, name := range []string{"zebra", "alpha", "mike"} {
		cleanupKeyringEntry(t, name)
		if _, err := kb.Save(ctx, testConn(name)); err != nil {
			t.Fatalf("Save(%s): %v", name, err)
		}
	}

	list, err := kb.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"alpha", "mike", "zebra"}
	if len(list) != len(want) {
		t.Fatalf("List: got %d connections, want %d", len(list), len(want))
	}
	for i, w := range want {
		if list[i].Name != w {
			t.Fatalf("List[%d].Name = %q, want %q", i, list[i].Name, w)
		}
	}
}

func TestKeyringBackend_MetadataFileNeverContainsToken(t *testing.T) {
	dir := t.TempDir()
	kb := NewKeyringBackend(dir)
	conn := testConn("local")
	conn.Token = "unmistakably-secret-value-12345"
	cleanupKeyringEntry(t, "local")

	if _, err := kb.Save(context.Background(), conn); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "connections-meta.json"))
	if err != nil {
		t.Fatalf("reading metadata file: %v", err)
	}
	if bytes.Contains(raw, []byte(conn.Token)) {
		t.Fatalf("connections-meta.json contains the token value — it must not")
	}
}

func TestKeyringBackend_TokenRemovedExternally_GetReturnsErrTokenUnavailable(t *testing.T) {
	dir := t.TempDir()
	kb := NewKeyringBackend(dir)
	ctx := context.Background()
	cleanupKeyringEntry(t, "local") // belt and braces; test deletes it itself below

	if _, err := kb.Save(ctx, testConn("local")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Simulate the token having been removed from the keyring by
	// something other than xoluman (e.g. the OS's own keyring UI) —
	// metadata still says the connection exists, the keyring disagrees.
	if err := keyring.Delete(keyringService, "local"); err != nil {
		t.Fatalf("simulating external token removal: %v", err)
	}

	_, err := kb.Get(ctx, "local")
	if !errors.Is(err, ErrTokenUnavailable) {
		t.Fatalf("Get after external token removal: err = %v, want ErrTokenUnavailable", err)
	}
}

func TestKeyringBackend_TokenRemovedExternally_ListDegradesGracefully(t *testing.T) {
	dir := t.TempDir()
	kb := NewKeyringBackend(dir)
	ctx := context.Background()
	cleanupKeyringEntry(t, "local") // belt and braces; test deletes it itself below

	if _, err := kb.Save(ctx, testConn("local")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := keyring.Delete(keyringService, "local"); err != nil {
		t.Fatalf("simulating external token removal: %v", err)
	}

	list, err := kb.List(ctx)
	if err != nil {
		t.Fatalf("List: %v (want List to degrade, not fail)", err)
	}
	if len(list) != 1 {
		t.Fatalf("List: got %d connections, want 1", len(list))
	}
	if list[0].Token != "" {
		t.Fatalf("List[0].Token = %q, want empty (token unavailable)", list[0].Token)
	}
}

func TestKeyringBackend_SaveLeavesNoOrphanedMetadataOnKeyringFailure(t *testing.T) {
	dir := t.TempDir()
	kb := NewKeyringBackend(dir)

	// Force every keyring call to fail, simulating the exact failure
	// observed against a real environment with no keyring service
	// present (see docs/KNOWN_ISSUES.md's dormant-guard entry). Restored
	// via t.Cleanup so this doesn't leak into tests that run after it.
	simulated := errors.New("simulated keyring failure")
	keyring.MockInitWithError(simulated)
	t.Cleanup(func() { keyring.MockInit() })

	_, err := kb.Save(context.Background(), testConn("local"))
	if err == nil {
		t.Fatal("Save: want an error when the keyring write fails, got nil")
	}

	// Save's contract: keyring is written before metadata, so a keyring
	// failure must leave no metadata file at all — not a file describing
	// a connection with no token.
	if _, statErr := os.Stat(filepath.Join(dir, "connections-meta.json")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("connections-meta.json exists after a keyring write failure, want absent (statErr=%v)", statErr)
	}
}

func TestKeyringBackend_SaveRollsBackKeyringOnMetadataWriteFailure(t *testing.T) {
	// Point at a directory that does not exist and is never created —
	// keyring.Set (mocked) succeeds first as normal, then saveMeta's
	// os.WriteFile fails because the parent directory is absent. Save's
	// rollback path should then delete the keyring entry it just wrote,
	// so nothing is left half-persisted in either store.
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	kb := NewKeyringBackend(dir)
	cleanupKeyringEntry(t, "local") // belt and braces if rollback has a bug

	_, err := kb.Save(context.Background(), testConn("local"))
	if err == nil {
		t.Fatal("Save: want an error when the metadata directory does not exist, got nil")
	}

	if _, kerr := keyring.Get(keyringService, "local"); !errors.Is(kerr, keyring.ErrNotFound) {
		t.Fatalf("keyring.Get after failed Save: err = %v, want keyring.ErrNotFound (rollback should have removed it)", kerr)
	}
}

func TestKeyringBackend_SaveValidatesBeforeWriting(t *testing.T) {
	kb := NewKeyringBackend(t.TempDir())
	ctx := context.Background()

	if _, err := kb.Save(ctx, Connection{}); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Save(empty name): err = %v, want ErrInvalidName", err)
	}

	bad := testConn("local")
	bad.AuthMode = "not-a-real-mode"
	if _, err := kb.Save(ctx, bad); !errors.Is(err, ErrInvalidAuthMode) {
		t.Fatalf("Save(bad auth mode): err = %v, want ErrInvalidAuthMode", err)
	}

	// Neither invalid Save should have reached the keyring at all.
	if _, err := keyring.Get(keyringService, "local"); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("keyring has an entry after only-invalid saves, want none (err=%v)", err)
	}
}
