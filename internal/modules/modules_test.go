// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package modules

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegistry_AllReturnsRegisteredModules(t *testing.T) {
	r := NewRegistry()
	r.Register(Module{ID: "a", Label: "A"})
	r.Register(Module{ID: "b", Label: "B"})

	all := r.All()
	if len(all) != 2 {
		t.Fatalf("got %d modules, want 2", len(all))
	}
}

func TestRegistry_EmptyRegistryReturnsEmpty(t *testing.T) {
	r := NewRegistry()
	all := r.All()
	if len(all) != 0 {
		t.Fatalf("got %d modules, want 0", len(all))
	}
}

func TestRegistry_SortedByOrder(t *testing.T) {
	r := NewRegistry()
	r.Register(Module{ID: "third", Order: 30})
	r.Register(Module{ID: "first", Order: 10})
	r.Register(Module{ID: "second", Order: 20})

	all := r.All()
	want := []string{"first", "second", "third"}
	for i, w := range want {
		if all[i].ID != w {
			t.Fatalf("all[%d].ID = %q, want %q", i, all[i].ID, w)
		}
	}
}

func TestRegistry_EqualOrderPreservesRegistrationOrder(t *testing.T) {
	r := NewRegistry()
	r.Register(Module{ID: "registered-first", Order: 0})
	r.Register(Module{ID: "registered-second", Order: 0})
	r.Register(Module{ID: "registered-third", Order: 0})

	all := r.All()
	want := []string{"registered-first", "registered-second", "registered-third"}
	for i, w := range want {
		if all[i].ID != w {
			t.Fatalf("all[%d].ID = %q, want %q (equal Order must preserve registration order)", i, all[i].ID, w)
		}
	}
}

func TestRegistry_MixedOrderAndTies(t *testing.T) {
	r := NewRegistry()
	r.Register(Module{ID: "b-tied-1", Order: 10})
	r.Register(Module{ID: "a-first", Order: 0})
	r.Register(Module{ID: "b-tied-2", Order: 10})
	r.Register(Module{ID: "c-last", Order: 20})

	all := r.All()
	want := []string{"a-first", "b-tied-1", "b-tied-2", "c-last"}
	for i, w := range want {
		if all[i].ID != w {
			t.Fatalf("all[%d].ID = %q, want %q", i, all[i].ID, w)
		}
	}
}

func TestRegistry_AllReturnsACopyNotTheInternalSlice(t *testing.T) {
	r := NewRegistry()
	r.Register(Module{ID: "a"})

	got := r.All()
	got[0].ID = "mutated"

	again := r.All()
	if again[0].ID != "a" {
		t.Fatalf("mutating All()'s result affected the registry: got %q, want %q", again[0].ID, "a")
	}
}

func TestRegistry_VisibleFiltersByPredicate(t *testing.T) {
	r := NewRegistry()
	r.Register(Module{ID: "visible", Order: 0})
	r.Register(Module{ID: "hidden", Order: 1})

	visible := r.Visible(func(m Module) bool { return m.ID == "visible" })
	if len(visible) != 1 || visible[0].ID != "visible" {
		t.Fatalf("Visible = %v, want exactly [visible]", visible)
	}
}

func TestRegistry_VisibleWithAlwaysTrueMatchesAll(t *testing.T) {
	r := NewRegistry()
	r.Register(Module{ID: "a"})
	r.Register(Module{ID: "b"})

	visible := r.Visible(func(Module) bool { return true })
	if len(visible) != 2 {
		t.Fatalf("got %d modules, want 2", len(visible))
	}
}

func TestRegistry_VisiblePreservesOrder(t *testing.T) {
	r := NewRegistry()
	r.Register(Module{ID: "second", Order: 10})
	r.Register(Module{ID: "first", Order: 0})

	visible := r.Visible(func(Module) bool { return true })
	if visible[0].ID != "first" || visible[1].ID != "second" {
		t.Fatalf("Visible order = %v, want [first, second]", visible)
	}
}

func TestRegistry_MountAll_CallsEveryNonNilMountRoutes(t *testing.T) {
	r := NewRegistry()
	var mounted []string
	r.Register(Module{ID: "a", MountRoutes: func(mux *http.ServeMux) {
		mounted = append(mounted, "a")
		mux.HandleFunc("GET /a", func(w http.ResponseWriter, r *http.Request) {})
	}})
	r.Register(Module{ID: "b", MountRoutes: func(mux *http.ServeMux) {
		mounted = append(mounted, "b")
		mux.HandleFunc("GET /b", func(w http.ResponseWriter, r *http.Request) {})
	}})

	mux := http.NewServeMux()
	r.MountAll(mux)

	if len(mounted) != 2 || mounted[0] != "a" || mounted[1] != "b" {
		t.Fatalf("mounted = %v, want [a b]", mounted)
	}
}

func TestRegistry_MountAll_SkipsNilMountRoutesWithoutPanicking(t *testing.T) {
	r := NewRegistry()
	r.Register(Module{ID: "no-routes"}) // MountRoutes deliberately nil
	called := false
	r.Register(Module{ID: "has-routes", MountRoutes: func(mux *http.ServeMux) { called = true }})

	mux := http.NewServeMux()
	r.MountAll(mux) // must not panic on the nil MountRoutes

	if !called {
		t.Fatal("MountAll did not call the module that does have MountRoutes")
	}
}

func TestRegistry_MountAll_RoutesActuallyWorkThroughTheMux(t *testing.T) {
	r := NewRegistry()
	r.Register(Module{ID: "a", MountRoutes: func(mux *http.ServeMux) {
		mux.HandleFunc("GET /probe", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("mounted"))
		})
	}})

	mux := http.NewServeMux()
	r.MountAll(mux)

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Body.String() != "mounted" {
		t.Fatalf("body = %q, want %q — MountAll's routes must genuinely work through the mux, not just be called", rec.Body.String(), "mounted")
	}
}
