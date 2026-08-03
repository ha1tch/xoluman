// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package modules is xoluman's module registry: self-registering
// feature areas, with both the top nav and the route table built from
// whatever's registered rather than hand-listed per page or wired
// centrally in internal/server/server.go. Adapted from Seam AMS's own
// internal/modules — same registration and MountRoutes pattern,
// deliberately without Seam's role-based VisibleTo(role) gating, since
// xoluman has no multi-user/role model today. Registry.Visible takes a
// predicate rather than nothing, so a visibility layer can be added
// later without changing every call site — it just isn't built yet.
package modules

import (
	"net/http"
	"sync"
)

// Module is one registered feature area — Connections, Entities, and
// now FSM definitions, with a graph editor and a full FSM editor
// confirmed as planned (see docs/TRACKING.md, T-08/T-14). Three or more
// distinct areas each wiring their own routes by hand in server.go is
// exactly what stops scaling; MountRoutes is the fix.
type Module struct {
	// ID identifies the module for lookups; not shown in the UI.
	ID string
	// Label is the nav link's visible text.
	Label string
	// URL is the nav link's target.
	URL string
	// ActivePrefix marks the nav link active when the current request
	// path starts with this prefix. Separate from URL because a
	// module's active range is usually broader than its single entry
	// link (e.g. every /connections/... path, not just /connections
	// itself).
	ActivePrefix string
	// Order controls nav position, ascending. Modules with equal Order
	// fall back to registration order.
	Order int
	// MountRoutes registers this module's own routes onto the shared
	// top-level mux. May be nil for a module that only wants a nav
	// entry (rare) or is still being scaffolded — MountAll skips a nil
	// MountRoutes rather than panicking.
	MountRoutes func(mux *http.ServeMux)
}

// Registry holds registered modules. The zero value is not usable —
// construct with NewRegistry.
type Registry struct {
	mu      sync.Mutex
	modules []Module
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds m to the registry. Safe to call from an init() —
// that's the expected usage: whichever package owns a feature area
// registers itself, rather than every page hand-listing every nav
// entry or server.go hand-wiring every route.
func (r *Registry) Register(m Module) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modules = append(r.modules, m)
}

// All returns every registered module, sorted by Order (registration
// order breaks ties within equal Order, via a stable sort).
func (r *Registry) All() []Module {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Module, len(r.modules))
	copy(out, r.modules)
	stableSortByOrder(out)
	return out
}

// Visible returns every registered module for which include returns
// true, in the same Order-then-registration-order as All. This is the
// hook a future visibility layer plugs into — e.g. Visible(func(m
// Module) bool { return roleCanSee(currentRole, m) }) — without needing
// to change how callers currently use All() for everything, or without
// building that predicate's logic now. No caller passes a real
// predicate yet; xoluman has no multi-user/role model to gate on.
func (r *Registry) Visible(include func(Module) bool) []Module {
	all := r.All()
	out := make([]Module, 0, len(all))
	for _, m := range all {
		if include(m) {
			out = append(out, m)
		}
	}
	return out
}

// MountAll calls MountRoutes on every registered module with a non-nil
// one, in registration order — server.go's own routing for a module
// shrinks to just this one call plus whatever isn't a module (static
// asset serving, the root redirect).
func (r *Registry) MountAll(mux *http.ServeMux) {
	r.mu.Lock()
	mods := make([]Module, len(r.modules))
	copy(mods, r.modules)
	r.mu.Unlock()

	for _, m := range mods {
		if m.MountRoutes != nil {
			m.MountRoutes(mux)
		}
	}
}

// stableSortByOrder is a tiny insertion sort — registries hold a
// handful of modules, not enough to justify sort.Slice's overhead or
// importing "sort" for something this small.
func stableSortByOrder(modules []Module) {
	for i := 1; i < len(modules); i++ {
		j := i
		for j > 0 && modules[j-1].Order > modules[j].Order {
			modules[j-1], modules[j] = modules[j], modules[j-1]
			j--
		}
	}
}
