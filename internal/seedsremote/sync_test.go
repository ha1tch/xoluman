// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package seedsremote

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// zipEntry is one file or directory to build into a test zip archive.
type zipEntry struct {
	name string // full path within the archive, GitHub-wrapper prefix included
	dir  bool
	body string
}

// buildZip produces a real, valid .zip from entries — used to build
// both well-formed fixtures and deliberately malicious ones (path
// traversal) without depending on any external tool.
func buildZip(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		name := e.name
		if e.dir && name[len(name)-1] != '/' {
			name += "/"
		}
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("creating zip entry %s: %v", name, err)
		}
		if !e.dir {
			if _, err := w.Write([]byte(e.body)); err != nil {
				t.Fatalf("writing zip body for %s: %v", name, err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("closing zip writer: %v", err)
	}
	return buf.Bytes()
}

func serveZip(t *testing.T, data []byte) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func TestSync_ExtractsRealSeedPackage(t *testing.T) {
	data := buildZip(t, []zipEntry{
		{name: "ha1tch-xoluseeds-abc1234/", dir: true},
		{name: "ha1tch-xoluseeds-abc1234/hello-fsm/", dir: true},
		{name: "ha1tch-xoluseeds-abc1234/hello-fsm/seed.json", body: `{"format_version":1,"id":"hello-fsm","name":"Hello FSM","steps":[{"type":"fsm","file":"fsms/a.json"}]}`},
		{name: "ha1tch-xoluseeds-abc1234/hello-fsm/fsms/", dir: true},
		{name: "ha1tch-xoluseeds-abc1234/hello-fsm/fsms/a.json", body: `{"name":"x"}`},
	})
	url := serveZip(t, data)
	dest := filepath.Join(t.TempDir(), "cache")

	if err := Sync(context.Background(), url, dest); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	manifestPath := filepath.Join(dest, "hello-fsm", "seed.json")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("reading extracted manifest: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("extracted manifest is empty")
	}
	fsmPath := filepath.Join(dest, "hello-fsm", "fsms", "a.json")
	if _, err := os.Stat(fsmPath); err != nil {
		t.Fatalf("extracted fsm file missing: %v", err)
	}
}

func TestSync_EmptyRepoIsNotAnError(t *testing.T) {
	// Mirrors ha1tch/xoluseeds' own real, current state: just the
	// wrapper directory entry, nothing inside it.
	data := buildZip(t, []zipEntry{
		{name: "ha1tch-xoluseeds-abc1234/", dir: true},
	})
	url := serveZip(t, data)
	dest := filepath.Join(t.TempDir(), "cache")

	if err := Sync(context.Background(), url, dest); err != nil {
		t.Fatalf("Sync: %v, want no error for an empty repository", err)
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatalf("reading dest dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("dest dir has %d entries, want 0 for an empty repository", len(entries))
	}
}

func TestSync_PathTraversalRejected(t *testing.T) {
	data := buildZip(t, []zipEntry{
		{name: "ha1tch-xoluseeds-abc1234/", dir: true},
		{name: "ha1tch-xoluseeds-abc1234/../../../etc/evil.json", body: "malicious content"},
	})
	url := serveZip(t, data)
	dest := filepath.Join(t.TempDir(), "cache")

	err := Sync(context.Background(), url, dest)
	if err == nil {
		t.Fatal("expected an error for a path-traversal zip entry, got none")
	}

	// Confirm the malicious file was never actually written anywhere
	// on disk, not just that Sync returned an error.
	suspect := filepath.Join(filepath.Dir(filepath.Dir(dest)), "etc", "evil.json")
	if _, statErr := os.Stat(suspect); statErr == nil {
		t.Fatalf("the path-traversal entry was actually written to %s", suspect)
	}
}

func TestSync_ClearsStaleContentFromPriorSync(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "cache")

	firstData := buildZip(t, []zipEntry{
		{name: "w/", dir: true},
		{name: "w/old-seed/", dir: true},
		{name: "w/old-seed/seed.json", body: `{"id":"old-seed"}`},
	})
	if err := Sync(context.Background(), serveZip(t, firstData), dest); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "old-seed")); err != nil {
		t.Fatalf("first sync's own content missing: %v", err)
	}

	secondData := buildZip(t, []zipEntry{
		{name: "w/", dir: true},
		{name: "w/new-seed/", dir: true},
		{name: "w/new-seed/seed.json", body: `{"id":"new-seed"}`},
	})
	if err := Sync(context.Background(), serveZip(t, secondData), dest); err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dest, "old-seed")); err == nil {
		t.Fatal("old-seed from the prior sync is still present, want it cleared")
	}
	if _, err := os.Stat(filepath.Join(dest, "new-seed")); err != nil {
		t.Fatalf("new-seed from the second sync is missing: %v", err)
	}
}

func TestSync_HTTPErrorPropagates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	dest := filepath.Join(t.TempDir(), "cache")

	err := Sync(context.Background(), server.URL, dest)
	if err == nil {
		t.Fatal("expected an error for a 404 response, got none")
	}
}

func TestSync_InvalidZipPropagates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not actually zip data"))
	}))
	t.Cleanup(server.Close)
	dest := filepath.Join(t.TempDir(), "cache")

	err := Sync(context.Background(), server.URL, dest)
	if err == nil {
		t.Fatal("expected an error for invalid zip content, got none")
	}
}
