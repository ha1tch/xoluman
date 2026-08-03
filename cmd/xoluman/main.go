// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Command xoluman is a web-based operator UI for xolu — connection
// management, data browsing/editing, query running, and blob access,
// against one or more local or remote xolu instances.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/ha1tch/xoluman/internal/config"
	"github.com/ha1tch/xoluman/internal/connstore"
	"github.com/ha1tch/xoluman/internal/server"
)

func main() {
	addr := flag.String("addr", ":8090", "address to listen on")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	settings, err := config.Load()
	if err != nil {
		logger.Error("loading settings", "error", err)
		os.Exit(1)
	}

	store, err := buildStore(settings)
	if err != nil {
		logger.Error("initializing connection store", "error", err)
		os.Exit(1)
	}

	dir, err := config.Dir()
	if err != nil {
		logger.Error("resolving config directory", "error", err)
		os.Exit(1)
	}
	logger.Info("xoluman starting",
		"addr", *addr,
		"secret_backend", settings.SecretBackend,
		"config_dir", dir,
	)

	handler := server.New(store)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}

// buildStore constructs the connstore.Store matching settings' chosen
// backend. The backend choice is made once at first run — see
// docs/KNOWN_ISSUES.md's recorded decision — not re-decided per launch.
func buildStore(settings config.Settings) (connstore.Store, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, fmt.Errorf("resolving config directory: %w", err)
	}

	switch settings.SecretBackend {
	case config.SecretBackendKeyring:
		return connstore.NewKeyringBackend(dir), nil
	case config.SecretBackendFile:
		return connstore.NewFileBackend(dir), nil
	default:
		// config.Load already defends against an unrecognised value by
		// falling back to SecretBackendFile, so this is unreachable in
		// practice — kept as an explicit error rather than a silent
		// default so a future change to that fallback can't produce a
		// store nobody chose.
		return nil, fmt.Errorf("unrecognised secret backend %q", settings.SecretBackend)
	}
}
