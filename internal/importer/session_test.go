// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package importer

import (
	"testing"
	"time"
)

func TestSessionStore_PutThenTake(t *testing.T) {
	store := NewSessionStore()
	sess := Session{ConnName: "local", EntityType: "widgets", Rows: []Row{{Index: 1}}}

	id, err := store.Put(sess)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if id == "" {
		t.Fatal("Put returned an empty id")
	}

	got, ok := store.Take(id)
	if !ok {
		t.Fatal("Take: want ok=true for a freshly-put session")
	}
	if got.ConnName != "local" || got.EntityType != "widgets" || len(got.Rows) != 1 {
		t.Fatalf("got = %+v, unexpected", got)
	}
}

func TestSessionStore_TakeIsSingleUse(t *testing.T) {
	store := NewSessionStore()
	id, err := store.Put(Session{ConnName: "local"})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if _, ok := store.Take(id); !ok {
		t.Fatal("first Take: want ok=true")
	}
	if _, ok := store.Take(id); ok {
		t.Fatal("second Take on the same id: want ok=false, sessions are single-use")
	}
}

func TestSessionStore_TakeUnknownID(t *testing.T) {
	store := NewSessionStore()
	if _, ok := store.Take("does-not-exist"); ok {
		t.Fatal("Take on an unknown id: want ok=false")
	}
}

func TestSessionStore_PutGeneratesDistinctIDs(t *testing.T) {
	store := NewSessionStore()
	id1, err := store.Put(Session{ConnName: "a"})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	id2, err := store.Put(Session{ConnName: "b"})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if id1 == id2 {
		t.Fatalf("two Put calls returned the same id %q", id1)
	}
}

func TestSessionStore_ExpiredSessionNotReturned(t *testing.T) {
	store := NewSessionStore()
	// Bypass Put (which always stamps createdAt = now) to directly
	// install an already-expired session, same package so the
	// unexported field is reachable — this is exactly what a real
	// expired entry looks like after sessionMaxAge has passed.
	store.sessions["expired-id"] = Session{
		ConnName:  "local",
		createdAt: time.Now().Add(-sessionMaxAge - time.Minute),
	}

	if _, ok := store.Take("expired-id"); ok {
		t.Fatal("Take on an expired session: want ok=false")
	}
}

func TestSessionStore_ExpiredSessionSweptOnPut(t *testing.T) {
	store := NewSessionStore()
	store.sessions["expired-id"] = Session{
		ConnName:  "local",
		createdAt: time.Now().Add(-sessionMaxAge - time.Minute),
	}

	if _, err := store.Put(Session{ConnName: "fresh"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if _, present := store.sessions["expired-id"]; present {
		t.Fatal("expired session still present after Put, want it swept")
	}
}

func TestSessionStore_FreshSessionSurvivesSweep(t *testing.T) {
	store := NewSessionStore()
	id, err := store.Put(Session{ConnName: "fresh"})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Trigger a sweep via a second Put — the fresh session from above
	// must not be collateral damage.
	if _, err := store.Put(Session{ConnName: "other"}); err != nil {
		t.Fatalf("second Put: %v", err)
	}

	if _, ok := store.Take(id); !ok {
		t.Fatal("Take on the still-fresh session: want ok=true, a sweep must not remove non-expired entries")
	}
}
