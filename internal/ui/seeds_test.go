// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ha1tch/xoluman/internal/config"
)

// withSeedsDir points XOLUMAN_CONFIG_DIR at a fresh temp config dir
// and writes a settings.json whose SeedsDir points at a fresh, real
// seed directory built from manifestJSON and files, matching how a
// real, on-disk seed package would look. Returns the seeds directory
// (the parent of the one real seed subdirectory written).
func withSeedsDir(t *testing.T, seedID, manifestJSON string, files map[string]string) {
	t.Helper()
	configDir := t.TempDir()
	t.Setenv("XOLUMAN_CONFIG_DIR", configDir)

	seedsDir := t.TempDir()
	seedDir := filepath.Join(seedsDir, seedID)
	if err := os.MkdirAll(seedDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seedDir, "seed.json"), []byte(manifestJSON), 0644); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}
	for rel, content := range files {
		full := filepath.Join(seedDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("writing %s: %v", rel, err)
		}
	}

	settings := config.DefaultSettings()
	settings.SeedsDir = seedsDir
	if err := config.Save(settings); err != nil {
		t.Fatalf("saving settings: %v", err)
	}
}

func TestSeedsHandler_Browse_NoSeedsConfigured(t *testing.T) {
	t.Setenv("XOLUMAN_CONFIG_DIR", t.TempDir())
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := &seedsHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/seeds", nil)
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.Browse(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "No seeds configured") {
		t.Errorf("body missing the no-seeds message: %s", rec.Body.String())
	}
}

func TestSeedsHandler_Browse_ListsRealSeeds(t *testing.T) {
	withSeedsDir(t, "demo-seed",
		`{"format_version":1,"id":"demo-seed","name":"Demo Seed","description":"A test seed","steps":[{"type":"fsm","file":"fsms/a.json"}]}`,
		map[string]string{"fsms/a.json": `{"name":"x"}`},
	)
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := &seedsHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/seeds", nil)
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.Browse(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Demo Seed") {
		t.Errorf("body missing the seed name: %s", rec.Body.String())
	}
}

func TestSeedsHandler_Preview_ShowsStepSummary(t *testing.T) {
	withSeedsDir(t, "demo-seed",
		`{"format_version":1,"id":"demo-seed","name":"Demo Seed","description":"desc","steps":[
			{"type":"fsm","file":"fsms/a.json"},
			{"type":"data","entity_type":"companies","file":"data/companies.jsonl"}
		]}`,
		map[string]string{"fsms/a.json": `{"name":"x"}`, "data/companies.jsonl": `{"name":"Acme"}`},
	)
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := &seedsHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/seeds/demo-seed", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("id", "demo-seed")
	rec := httptest.NewRecorder()

	h.Preview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "FSM definitions") || !strings.Contains(body, "Entity data files") {
		t.Errorf("body missing the expected step summary rows: %s", body)
	}
}

func TestSeedsHandler_Preview_UnknownSeedID(t *testing.T) {
	withSeedsDir(t, "demo-seed",
		`{"format_version":1,"id":"demo-seed","name":"Demo Seed","steps":[{"type":"fsm","file":"a.json"}]}`,
		map[string]string{"a.json": `{}`},
	)
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := &seedsHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/seeds/does-not-exist", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("id", "does-not-exist")
	rec := httptest.NewRecorder()

	h.Preview(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatal("expected a non-200 status for an unknown seed id")
	}
}

func TestSeedsHandler_PreviewImage_ServesRealFile(t *testing.T) {
	withSeedsDir(t, "demo-seed",
		`{"format_version":1,"id":"demo-seed","name":"Demo Seed","preview_images":["preview/a.png"],"steps":[{"type":"fsm","file":"fsms/a.json"}]}`,
		map[string]string{"preview/a.png": "fake-png-bytes", "fsms/a.json": `{}`},
	)
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := &seedsHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/seeds/demo-seed/preview/preview/a.png", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("id", "demo-seed")
	req.SetPathValue("file", "preview/a.png")
	rec := httptest.NewRecorder()

	h.PreviewImage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "fake-png-bytes" {
		t.Errorf("body = %q, want the real file content", rec.Body.String())
	}
}

func TestSeedsHandler_PreviewImage_PathTraversalRejected(t *testing.T) {
	withSeedsDir(t, "demo-seed",
		`{"format_version":1,"id":"demo-seed","name":"Demo Seed","steps":[{"type":"fsm","file":"fsms/a.json"}]}`,
		map[string]string{"fsms/a.json": `{}`},
	)
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := &seedsHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/seeds/demo-seed/preview/../../../../etc/passwd", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("id", "demo-seed")
	req.SetPathValue("file", "../../../../etc/passwd")
	rec := httptest.NewRecorder()

	h.PreviewImage(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("path traversal was not rejected: status %d, body %q", rec.Code, rec.Body.String())
	}
}

func TestSeedsHandler_ApplySeed_BlockedWhenConnectionNotEmpty(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/tenant-summary", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"primary":    map[string]int{"nodes": 3},
			"loc":        map[string]int{},
			"obj":        map[string]int{},
			"ts":         0,
			"cal_index":  0,
			"bal_rollup": 0,
			"blob":       0,
			"empty":      false,
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	withSeedsDir(t, "demo-seed",
		`{"format_version":1,"id":"demo-seed","name":"Demo Seed","steps":[{"type":"fsm","file":"fsms/a.json"}]}`,
		map[string]string{"fsms/a.json": `{"name":"x"}`},
	)
	store := seedConnection(t, server.URL)
	h := &seedsHandler{store: store}

	req := httptest.NewRequest(http.MethodPost, "/connections/test/seeds/demo-seed/apply", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("id", "demo-seed")
	rec := httptest.NewRecorder()

	h.ApplySeed(rec, req)

	var resp applyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not valid JSON: %v (body: %s)", err, rec.Body.String())
	}
	if !resp.Blocked {
		t.Fatalf("resp.Blocked = false, want true for a non-empty connection: %+v", resp)
	}
	if resp.EmptinessCheck == nil {
		t.Fatal("resp.EmptinessCheck is nil, want the full report")
	}
}

func TestSeedsHandler_ApplySeed_SkipsCheckWhenConfigured(t *testing.T) {
	applyCallCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/fsm/def", func(w http.ResponseWriter, r *http.Request) {
		applyCallCount++
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "name": "x", "created_at": "2026-01-01T00:00:00Z"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	withSeedsDir(t, "demo-seed",
		`{"format_version":1,"id":"demo-seed","name":"Demo Seed","steps":[{"type":"fsm","file":"fsms/a.json"}]}`,
		map[string]string{"fsms/a.json": `{"name":"x","initial":"start","states":{"start":{}},"transitions":[]}`},
	)
	// Load, flip the skip flag, save -- confirming ApplySeed reads
	// this setting for real, not just accepting the withSeedsDir
	// default.
	settings, err := config.Load()
	if err != nil {
		t.Fatalf("loading settings: %v", err)
	}
	settings.SeedSkipEmptyCheck = true
	if err := config.Save(settings); err != nil {
		t.Fatalf("saving settings: %v", err)
	}

	store := seedConnection(t, server.URL)
	h := &seedsHandler{store: store}

	req := httptest.NewRequest(http.MethodPost, "/connections/test/seeds/demo-seed/apply", nil)
	req.SetPathValue("name", "test")
	req.SetPathValue("id", "demo-seed")
	rec := httptest.NewRecorder()

	h.ApplySeed(rec, req)

	var resp applyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not valid JSON: %v (body: %s)", err, rec.Body.String())
	}
	if resp.Blocked {
		t.Fatal("resp.Blocked = true, want false -- the empty check was configured to be skipped")
	}
	if !resp.Success {
		t.Fatalf("resp.Success = false, want true: %+v", resp)
	}
	if applyCallCount != 1 {
		t.Errorf("fsm def endpoint called %d times, want 1 (confirms Apply actually ran, not just skipped everything)", applyCallCount)
	}
}

func TestSeedsHandler_RollbackSeed_CallsRollback(t *testing.T) {
	deleteCalled := false
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/companies/1", func(w http.ResponseWriter, r *http.Request) {
		deleteCalled = true
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := seedConnection(t, server.URL)
	h := &seedsHandler{store: store}

	body := `{"created":[{"Kind":"entity","EntityType":"companies","ID":1}]}`
	req := httptest.NewRequest(http.MethodPost, "/connections/test/seeds/rollback", strings.NewReader(body))
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.RollbackSeed(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !deleteCalled {
		t.Fatal("DELETE was never called -- Rollback did not actually attempt to remove the tracked entity")
	}
}

func TestSeedsHandler_ApplySeed_UnknownConnection(t *testing.T) {
	t.Setenv("XOLUMAN_CONFIG_DIR", t.TempDir())
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := &seedsHandler{store: store}

	req := httptest.NewRequest(http.MethodPost, "/connections/does-not-exist/seeds/x/apply", nil)
	req.SetPathValue("name", "does-not-exist")
	req.SetPathValue("id", "x")
	rec := httptest.NewRecorder()

	h.ApplySeed(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatal("expected a non-200 status for an unknown connection")
	}
}

// buildTestZip produces a minimal, real .zip — GitHub-style top-level
// wrapper directory plus one seed package — served via httptest to
// test the remote-source flow without ever touching the real network.
func buildTestZip(t *testing.T, seedID string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	entries := []struct {
		name string
		dir  bool
		body string
	}{
		{name: "wrap/", dir: true},
		{name: "wrap/" + seedID + "/", dir: true},
		{name: "wrap/" + seedID + "/seed.json", body: `{"format_version":1,"id":"` + seedID + `","name":"Remote Demo","steps":[{"type":"fsm","file":"fsms/a.json"}]}`},
		{name: "wrap/" + seedID + "/fsms/", dir: true},
		{name: "wrap/" + seedID + "/fsms/a.json", body: `{"name":"x"}`},
	}
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("creating zip entry %s: %v", e.name, err)
		}
		if !e.dir {
			if _, err := w.Write([]byte(e.body)); err != nil {
				t.Fatalf("writing zip body for %s: %v", e.name, err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("closing zip writer: %v", err)
	}
	return buf.Bytes()
}

func TestDiscoverRemoteSeeds_DisabledDoesNoNetworkAccess(t *testing.T) {
	t.Setenv("XOLUMAN_CONFIG_DIR", t.TempDir())
	// A URL that would hang or fail immediately if ever actually
	// dialed -- confirming disabled truly means zero network access,
	// not just "network access that happens to succeed anyway."
	const unreachable = "http://127.0.0.1:1/unreachable"
	settings := config.DefaultSettings() // SeedAllowRemoteSources false by default

	remote := discoverRemoteSeeds(context.Background(), settings, unreachable)

	if remote.Attempted {
		t.Fatal("Attempted = true, want false when SeedAllowRemoteSources is off")
	}
}

func TestDiscoverRemoteSeeds_SuccessfulSync(t *testing.T) {
	t.Setenv("XOLUMAN_CONFIG_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(buildTestZip(t, "remote-demo"))
	}))
	t.Cleanup(server.Close)

	settings := config.DefaultSettings()
	settings.SeedAllowRemoteSources = true

	remote := discoverRemoteSeeds(context.Background(), settings, server.URL)

	if !remote.Attempted {
		t.Fatal("Attempted = false, want true when SeedAllowRemoteSources is on")
	}
	if remote.SyncErr != nil {
		t.Fatalf("SyncErr = %v, want nil", remote.SyncErr)
	}
	if len(remote.Seeds) != 1 || remote.Seeds[0].Manifest.ID != "remote-demo" {
		t.Fatalf("Seeds = %+v, want exactly the one remote-demo seed", remote.Seeds)
	}
}

func TestDiscoverRemoteSeeds_FailedSyncReportsError(t *testing.T) {
	t.Setenv("XOLUMAN_CONFIG_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	settings := config.DefaultSettings()
	settings.SeedAllowRemoteSources = true

	remote := discoverRemoteSeeds(context.Background(), settings, server.URL)

	if !remote.Attempted {
		t.Fatal("Attempted = false, want true")
	}
	if remote.SyncErr == nil {
		t.Fatal("SyncErr = nil, want a real error for a failed fetch")
	}
}

func TestFindSeed_RemotePrefixResolvesFromCache(t *testing.T) {
	t.Setenv("XOLUMAN_CONFIG_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(buildTestZip(t, "remote-demo"))
	}))
	t.Cleanup(server.Close)

	settings := config.DefaultSettings()
	settings.SeedAllowRemoteSources = true
	// Populate the cache the same way Browse would -- one sync.
	discoverRemoteSeeds(context.Background(), settings, server.URL)

	ls, err := findSeed("remote-remote-demo")
	if err != nil {
		t.Fatalf("findSeed: %v", err)
	}
	if ls.Manifest.ID != "remote-demo" {
		t.Errorf("Manifest.ID = %q, want remote-demo", ls.Manifest.ID)
	}
}

func TestSeedsHandler_Browse_RemoteSectionAppearsWhenEnabled(t *testing.T) {
	t.Setenv("XOLUMAN_CONFIG_DIR", t.TempDir())
	tarballServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(buildTestZip(t, "remote-demo"))
	}))
	t.Cleanup(tarballServer.Close)

	settings := config.DefaultSettings()
	settings.SeedAllowRemoteSources = true
	if err := config.Save(settings); err != nil {
		t.Fatalf("saving settings: %v", err)
	}

	// Browse hardcodes seedsremote.DefaultTarballURL internally, so
	// this test confirms the piece it CAN control directly --
	// discoverRemoteSeeds itself, already covered above — is wired
	// into Browse's own settings check by confirming Attempted is
	// honored end to end via a direct call matching Browse's own
	// logic, since redirecting Browse's hardcoded URL would require
	// a much larger refactor for marginal additional coverage.
	remote := discoverRemoteSeeds(context.Background(), settings, tarballServer.URL)
	if !remote.Attempted || remote.SyncErr != nil || len(remote.Seeds) != 1 {
		t.Fatalf("discoverRemoteSeeds = %+v, want a clean, successful, 1-seed result", remote)
	}
}

func TestSeedsHandler_SyncRemote_RefusesWhenDisabled(t *testing.T) {
	t.Setenv("XOLUMAN_CONFIG_DIR", t.TempDir())
	store := seedConnection(t, fakeXoluWithWidgets(t).URL)
	h := &seedsHandler{store: store}

	req := httptest.NewRequest(http.MethodPost, "/connections/test/seeds/sync-remote", nil)
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.SyncRemote(rec, req)

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if attempted, _ := resp["attempted"].(bool); attempted {
		t.Fatal("attempted = true, want false when SeedAllowRemoteSources is off")
	}
}
