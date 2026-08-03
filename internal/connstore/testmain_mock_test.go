// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

//go:build !keyring_live

package connstore

import (
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestMain swaps the OS keyring provider for go-keyring's in-memory mock
// before any test in this package runs. This is what makes
// KeyringBackend's behavioural logic (upsert, hydrate, rollback, the
// metadata/token split) testable in the sandbox, which has no OS keyring
// service. It does not exercise the real OS keyring at all — that is
// testmain_live_test.go's job, under -tags keyring_live.
func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}
