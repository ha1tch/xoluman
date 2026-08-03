// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

//go:build keyring_live

package connstore

import (
	"os"
	"testing"
)

// TestMain here deliberately does nothing but run the suite — no
// keyring.MockInit() call, so go-keyring falls through to whatever real
// OS keyring provider it auto-detects (Secret Service on Linux, Keychain
// on macOS, Credential Manager on Windows). Every test in
// keyring_backend_test.go therefore becomes a genuine round-trip against
// that real provider. Build with -tags keyring_live; see the dormant
// guard entry in docs/KNOWN_ISSUES.md for the exact invocation.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
