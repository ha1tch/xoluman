// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package config resolves xoluman's on-disk configuration directory and
// reads/writes its app-level settings.json — currently just the secret
// storage backend selection (file vs keyring), made once at first run
// per docs/KNOWN_ISSUES.md's recorded decision.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// SecretBackend selects where connstore persists connection tokens.
type SecretBackend string

const (
	// SecretBackendFile stores tokens in plaintext JSON alongside
	// connection metadata (connections.json), permissions 0600. Always
	// available, the default.
	SecretBackendFile SecretBackend = "file"

	// SecretBackendKeyring stores tokens in the OS keyring; connection
	// metadata alone is written to JSON (connections-meta.json). Requires
	// an OS keyring service to be present — see T-04.
	SecretBackendKeyring SecretBackend = "keyring"
)

// Valid reports whether b is one of the two recognised backends.
func (b SecretBackend) Valid() bool {
	return b == SecretBackendFile || b == SecretBackendKeyring
}

// Settings is xoluman's app-level configuration, persisted to
// <Dir()>/settings.json.
type Settings struct {
	SecretBackend SecretBackend `json:"secret_backend"`
}

// DefaultSettings returns the settings xoluman starts with before any
// settings.json exists — SecretBackendFile, since it works everywhere
// with no external dependency.
func DefaultSettings() Settings {
	return Settings{SecretBackend: SecretBackendFile}
}

// Dir returns xoluman's configuration directory, creating it if absent.
// Resolution follows os.UserConfigDir() (XDG_CONFIG_HOME on Linux,
// ~/Library/Application Support on macOS, %AppData% on Windows), joined
// with "xoluman". XOLUMAN_CONFIG_DIR overrides this entirely when set —
// used by tests to avoid touching the real user config directory.
func Dir() (string, error) {
	if override := os.Getenv("XOLUMAN_CONFIG_DIR"); override != "" {
		if err := os.MkdirAll(override, 0o700); err != nil {
			return "", err
		}
		return override, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "xoluman")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// settingsPath returns <Dir()>/settings.json.
func settingsPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// Load reads settings.json, returning DefaultSettings() if it does not
// yet exist (first run — no backend has been chosen).
func Load() (Settings, error) {
	path, err := settingsPath()
	if err != nil {
		return Settings{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return Settings{}, err
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, err
	}
	if !s.SecretBackend.Valid() {
		s.SecretBackend = SecretBackendFile
	}
	return s, nil
}

// Save writes settings.json atomically (write to a temp file, then
// rename) so a crash mid-write never leaves a corrupt or partial file.
func Save(s Settings) error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
