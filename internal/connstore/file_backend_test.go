// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package connstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testConn(name string) Connection {
	return Connection{
		ConnectionMeta: ConnectionMeta{
			Name:     name,
			BaseURL:  "http://localhost:8080",
			AuthMode: AuthAPIKey,
			Tenant:   "",
		},
		Token: "secret-token-value",
	}
}

func TestFileBackend_SaveCreatesThenUpdates(t *testing.T) {
	dir := t.TempDir()
	fb := NewFileBackend(dir)
	ctx := context.Background()

	created, err := fb.Save(ctx, testConn("local"))
	if err != nil {
		t.Fatalf("Save (create): %v", err)
	}
	if !created {
		t.Fatal("Save (create): want created=true, got false")
	}

	updated := testConn("local")
	updated.BaseURL = "http://localhost:9090"
	created, err = fb.Save(ctx, updated)
	if err != nil {
		t.Fatalf("Save (update): %v", err)
	}
	if created {
		t.Fatal("Save (update): want created=false, got true")
	}

	got, err := fb.Get(ctx, "local")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.BaseURL != "http://localhost:9090" {
		t.Fatalf("Get after update: BaseURL = %q, want %q", got.BaseURL, "http://localhost:9090")
	}
}

func TestFileBackend_GetNotFound(t *testing.T) {
	fb := NewFileBackend(t.TempDir())
	_, err := fb.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(missing): err = %v, want ErrNotFound", err)
	}
}

func TestFileBackend_DeleteNotFound(t *testing.T) {
	fb := NewFileBackend(t.TempDir())
	err := fb.Delete(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(missing): err = %v, want ErrNotFound", err)
	}
}

func TestFileBackend_DeleteRemoves(t *testing.T) {
	dir := t.TempDir()
	fb := NewFileBackend(dir)
	ctx := context.Background()

	if _, err := fb.Save(ctx, testConn("local")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := fb.Delete(ctx, "local"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := fb.Get(ctx, "local"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete: err = %v, want ErrNotFound", err)
	}
}

func TestFileBackend_ListSortedByName(t *testing.T) {
	dir := t.TempDir()
	fb := NewFileBackend(dir)
	ctx := context.Background()

	for _, name := range []string{"zebra", "alpha", "mike"} {
		if _, err := fb.Save(ctx, testConn(name)); err != nil {
			t.Fatalf("Save(%s): %v", name, err)
		}
	}

	list, err := fb.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List: got %d connections, want 3", len(list))
	}
	want := []string{"alpha", "mike", "zebra"}
	for i, w := range want {
		if list[i].Name != w {
			t.Fatalf("List[%d].Name = %q, want %q", i, list[i].Name, w)
		}
	}
}

func TestFileBackend_SaveValidatesBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	fb := NewFileBackend(dir)
	ctx := context.Background()

	if _, err := fb.Save(ctx, Connection{}); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Save(empty name): err = %v, want ErrInvalidName", err)
	}

	bad := testConn("local")
	bad.AuthMode = "not-a-real-mode"
	if _, err := fb.Save(ctx, bad); !errors.Is(err, ErrInvalidAuthMode) {
		t.Fatalf("Save(bad auth mode): err = %v, want ErrInvalidAuthMode", err)
	}

	// Neither invalid Save should have created connections.json at all.
	if _, err := os.Stat(filepath.Join(dir, "connections.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("connections.json exists after only-invalid saves, want absent (err=%v)", err)
	}
}

func TestFileBackend_FilePermissionsAreOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	fb := NewFileBackend(dir)
	if _, err := fb.Save(context.Background(), testConn("local")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "connections.json"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("connections.json permissions = %o, want 0600", perm)
	}
}

func TestFileBackend_NoTempFileLeftAfterSave(t *testing.T) {
	dir := t.TempDir()
	fb := NewFileBackend(dir)
	if _, err := fb.Save(context.Background(), testConn("local")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "connections.json.tmp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temp file still present after successful Save (err=%v)", err)
	}
}

func TestFileBackend_PersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	fb1 := NewFileBackend(dir)
	if _, err := fb1.Save(ctx, testConn("local")); err != nil {
		t.Fatalf("Save via fb1: %v", err)
	}

	fb2 := NewFileBackend(dir)
	got, err := fb2.Get(ctx, "local")
	if err != nil {
		t.Fatalf("Get via fb2: %v", err)
	}
	if got.Token != "secret-token-value" {
		t.Fatalf("Get via fb2: Token = %q, want round-tripped value", got.Token)
	}
}

// Compile-time assertion that FileBackend implements Store.
var _ Store = (*FileBackend)(nil)
