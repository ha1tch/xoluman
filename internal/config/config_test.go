// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// withConfigDir points XOLUMAN_CONFIG_DIR at a fresh temp directory for
// the duration of the test, so these tests never touch the real user
// config directory.
func withConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XOLUMAN_CONFIG_DIR", dir)
	return dir
}

func TestLoad_DefaultsWhenNoSettingsFile(t *testing.T) {
	withConfigDir(t)

	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.SecretBackend != SecretBackendFile {
		t.Fatalf("SecretBackend = %q, want %q (default)", s.SecretBackend, SecretBackendFile)
	}
}

func TestSaveThenLoad_RoundTrips(t *testing.T) {
	withConfigDir(t)

	want := Settings{SecretBackend: SecretBackendKeyring}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.SecretBackend != want.SecretBackend {
		t.Fatalf("SecretBackend = %q, want %q", got.SecretBackend, want.SecretBackend)
	}
}

func TestLoad_InvalidBackendFallsBackToFile(t *testing.T) {
	dir := withConfigDir(t)

	// Write a settings.json with a nonsense backend value directly,
	// bypassing Save, to exercise Load's own defensive fallback.
	bad := []byte(`{"secret_backend":"not-a-real-backend"}`)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), bad, 0o600); err != nil {
		t.Fatalf("writing malformed settings.json: %v", err)
	}

	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.SecretBackend != SecretBackendFile {
		t.Fatalf("SecretBackend = %q, want fallback to %q", s.SecretBackend, SecretBackendFile)
	}
}

func TestDir_CreatesDirectory(t *testing.T) {
	// Deliberately point at a path that does not exist yet, one level
	// below a fresh temp dir, so Dir() has to create it.
	parent := t.TempDir()
	target := filepath.Join(parent, "does-not-exist-yet")
	t.Setenv("XOLUMAN_CONFIG_DIR", target)

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if got != target {
		t.Fatalf("Dir() = %q, want %q", got, target)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat(target): %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%q was created but is not a directory", target)
	}
}

func TestDir_UsesRealUserConfigDirWhenNoOverride(t *testing.T) {
	// Deliberately does not call withConfigDir — this is the one test
	// exercising the actual os.UserConfigDir() path rather than the
	// XOLUMAN_CONFIG_DIR override every other test uses. Points
	// XDG_CONFIG_HOME at a temp dir so it still never touches the real
	// user config directory.
	t.Setenv("XOLUMAN_CONFIG_DIR", "")
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	want := filepath.Join(xdg, "xoluman")
	if got != want {
		t.Fatalf("Dir() = %q, want %q", got, want)
	}
	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("Stat(got): %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%q was created but is not a directory", got)
	}
}

func TestDir_PropagatesMkdirFailure(t *testing.T) {
	// Point XOLUMAN_CONFIG_DIR at a path whose parent is a file, not a
	// directory — os.MkdirAll must fail, and Dir() must propagate that
	// rather than silently succeeding or panicking.
	parent := t.TempDir()
	blocker := filepath.Join(parent, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("creating blocker file: %v", err)
	}
	t.Setenv("XOLUMAN_CONFIG_DIR", filepath.Join(blocker, "xoluman"))

	if _, err := Dir(); err == nil {
		t.Fatal("Dir: want an error when the parent path is a file, got nil")
	}
}

func TestSaveSettingsFilePermissions(t *testing.T) {
	dir := withConfigDir(t)
	if err := Save(DefaultSettings()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("settings.json permissions = %o, want 0600", perm)
	}
}

func TestDefaultSettings_SeedFieldsAreSafeByDefault(t *testing.T) {
	s := DefaultSettings()
	if s.SeedSkipEmptyCheck {
		t.Error("SeedSkipEmptyCheck = true by default, want false (the empty check must run unless explicitly opted out)")
	}
	if s.SeedAllowRemoteSources {
		t.Error("SeedAllowRemoteSources = true by default, want false (remote seed sources are opt-in only)")
	}
}

// TestLoad_PreExistingSettingsFileWithoutSeedFieldsStaysSafe is the
// specific regression this design exists to prevent: a settings.json
// written before the seed-system fields existed (no "seed_skip_empty_
// check" or "seed_allow_remote_sources" key at all) must decode to
// the SAFE state for both — the check runs, remote sources stay off —
// never silently to the unsafe state. This is exactly why both fields
// are oriented so their zero value is the safe one; a naive
// "RequireEmptyConnection: true by default" field would fail this
// exact test, decoding to false (unsafe) for a file like this one.
func TestLoad_PreExistingSettingsFileWithoutSeedFieldsStaysSafe(t *testing.T) {
	dir := withConfigDir(t)
	oldFileContent := `{"secret_backend":"file"}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(oldFileContent), 0o600); err != nil {
		t.Fatalf("writing pre-existing settings.json: %v", err)
	}

	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.SeedSkipEmptyCheck {
		t.Error("SeedSkipEmptyCheck = true after loading a settings.json that predates this field, want false (safe)")
	}
	if s.SeedAllowRemoteSources {
		t.Error("SeedAllowRemoteSources = true after loading a settings.json that predates this field, want false (safe)")
	}
}

func TestSeedSettings_RoundTrip(t *testing.T) {
	withConfigDir(t)
	want := Settings{
		SecretBackend:          SecretBackendFile,
		SeedsDir:               "/some/path/to/seeds",
		SeedSkipEmptyCheck:     true,
		SeedAllowRemoteSources: true,
	}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
