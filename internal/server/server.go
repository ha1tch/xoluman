// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package server wires xoluman's HTTP routes to their handlers. Routing
// uses the standard library's method+pattern ServeMux (Go 1.22+) —
// there's no need for a router dependency at this scale, matching
// Seam's own choice not to pull one in for its UI layer either.
package server

import (
	"io/fs"
	"net/http"

	"github.com/ha1tch/xoluman/internal/connstore"
	"github.com/ha1tch/xoluman/internal/modules"
	"github.com/ha1tch/xoluman/internal/ui"
	"github.com/ha1tch/xoluman/web"
)

// New builds xoluman's complete route table against store and returns
// it as an http.Handler, ready to pass to http.ListenAndServe or
// httptest.NewServer.
//
// Each module registers its own routes (RegisterConnectionsModule,
// RegisterEntitiesModule, ...) into a registry built fresh for this
// call — not the shared, init()-populated one internal/ui uses for nav
// rendering. Reusing that shared registry here would accumulate one
// duplicate module entry per New call across a test binary, and
// http.ServeMux panics on a duplicate pattern registration; a fresh
// registry per call, mounted onto this call's own fresh mux, avoids
// that entirely. See internal/ui/connections.go's init() comment for
// the other half of this split.
func New(store connstore.Store) http.Handler {
	mux := http.NewServeMux()

	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		// web.Static's "static" directory is embedded at build time via
		// go:embed — its absence would be a build-time packaging defect,
		// not a runtime condition to recover from.
		panic("server: embedded static assets missing: " + err.Error())
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	reg := modules.NewRegistry()
	ui.RegisterConnectionsModule(reg, store)
	ui.RegisterEntitiesModule(reg, store)
	ui.RegisterBlobsModule(reg, store)
	ui.RegisterQueryModule(reg, store)
	ui.RegisterDXPModule(reg, store)
	ui.RegisterFSMModule(reg, store)
	ui.RegisterSeedsModule(reg, store)
	reg.MountAll(mux)
	ui.SetSidebarStore(store)

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/connections", http.StatusSeeOther)
	})

	return mux
}
