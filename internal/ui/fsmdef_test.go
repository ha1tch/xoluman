// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"encoding/json"
	"strings"
	"testing"

	xclient "github.com/ha1tch/xolu/pkg/client"
)

func rawFrom(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshaling from-value: %v", err)
	}
	return b
}

func TestFsmToolkitFSM_ConvertsStatesAndTransitions(t *testing.T) {
	spec := xclient.MachineSpec{
		Name:    "door",
		Initial: "closed",
		States:  map[string]xclient.StateDef{"closed": {}, "open": {}},
		Transitions: []xclient.TransitionDef{
			{From: rawFrom(t, "closed"), Input: "push", To: "open"},
			{From: rawFrom(t, "open"), Input: "close", To: "closed"},
		},
	}
	f, notes, err := fsmToolkitFSM(spec)
	if err != nil {
		t.Fatalf("fsmToolkitFSM: %v", err)
	}
	if len(notes) != 0 {
		t.Fatalf("notes = %v, want none for a spec with no xolu-specific features", notes)
	}
	if f.Initial != "closed" || !f.HasState("closed") || !f.HasState("open") {
		t.Fatalf("converted FSM missing expected states/initial: %+v", f)
	}
	if len(f.Transitions) != 2 {
		t.Fatalf("got %d transitions, want 2", len(f.Transitions))
	}
}

func TestFsmToolkitFSM_ExpandsMultiFromTransition(t *testing.T) {
	spec := xclient.MachineSpec{
		Name:    "m",
		Initial: "a",
		States:  map[string]xclient.StateDef{"a": {}, "b": {}, "c": {}},
		Transitions: []xclient.TransitionDef{
			{From: rawFrom(t, []string{"a", "b"}), Input: "go", To: "c"},
		},
	}
	f, _, err := fsmToolkitFSM(spec)
	if err != nil {
		t.Fatalf("fsmToolkitFSM: %v", err)
	}
	if len(f.Transitions) != 2 {
		t.Fatalf("got %d transitions, want 2 (one expanded per from-state)", len(f.Transitions))
	}
	froms := map[string]bool{}
	for _, tr := range f.Transitions {
		froms[tr.From] = true
	}
	if !froms["a"] || !froms["b"] {
		t.Fatalf("expanded transitions = %+v, want one from each of a and b", f.Transitions)
	}
}

func TestFsmToolkitFSM_NotesGuardAndSetAsSkipped(t *testing.T) {
	spec := xclient.MachineSpec{
		Name:    "m",
		Initial: "a",
		States:  map[string]xclient.StateDef{"a": {}, "b": {}},
		Transitions: []xclient.TransitionDef{
			{From: rawFrom(t, "a"), Input: "go", To: "b", Guard: "count > 0", Set: map[string]string{"count": "count - 1"}},
		},
	}
	_, notes, err := fsmToolkitFSM(spec)
	if err != nil {
		t.Fatalf("fsmToolkitFSM: %v", err)
	}
	joined := strings.Join(notes, " | ")
	if !strings.Contains(joined, "guard") {
		t.Fatalf("notes = %v, want a note about the unchecked guard", notes)
	}
	if !strings.Contains(joined, "variable assignment") {
		t.Fatalf("notes = %v, want a note about the unchecked Set clause", notes)
	}
}

func TestFsmToolkitFSM_NotesVariablesAndInputQueries(t *testing.T) {
	spec := xclient.MachineSpec{
		Name:         "m",
		Initial:      "a",
		States:       map[string]xclient.StateDef{"a": {}},
		Variables:    map[string]xclient.VariableDef{"count": {Type: "integer", Default: 0}},
		InputQueries: map[string]string{"tick": "SELECT 1"},
	}
	_, notes, err := fsmToolkitFSM(spec)
	if err != nil {
		t.Fatalf("fsmToolkitFSM: %v", err)
	}
	joined := strings.Join(notes, " | ")
	if !strings.Contains(joined, "variable") {
		t.Fatalf("notes = %v, want a note about the unchecked machine variable", notes)
	}
	if !strings.Contains(joined, "input quer") {
		t.Fatalf("notes = %v, want a note about the unchecked input query", notes)
	}
}

func TestFsmToolkitFSM_MalformedFromReturnsError(t *testing.T) {
	spec := xclient.MachineSpec{
		Name:    "m",
		Initial: "a",
		States:  map[string]xclient.StateDef{"a": {}},
		Transitions: []xclient.TransitionDef{
			{From: json.RawMessage(`{"not":"a state name or list"}`), Input: "go", To: "a"},
		},
	}
	_, _, err := fsmToolkitFSM(spec)
	if err == nil {
		t.Fatal("got nil error for a malformed From field, want an error")
	}
}

func TestValidateSpecLocally_CatchesMissingInitialState(t *testing.T) {
	spec := xclient.MachineSpec{
		Name:    "m",
		Initial: "nonexistent",
		States:  map[string]xclient.StateDef{"a": {}},
	}
	result := validateSpecLocally(spec)
	if result.Valid {
		t.Fatal("got valid=true for an initial state that doesn't exist among the declared states")
	}
	if len(result.Errors) == 0 {
		t.Fatal("want at least one error message")
	}
}

func TestValidateSpecLocally_CatchesDanglingTransitionTarget(t *testing.T) {
	spec := xclient.MachineSpec{
		Name:    "m",
		Initial: "a",
		States:  map[string]xclient.StateDef{"a": {}},
		Transitions: []xclient.TransitionDef{
			{From: rawFrom(t, "a"), Input: "go", To: "nonexistent"},
		},
	}
	result := validateSpecLocally(spec)
	if result.Valid {
		t.Fatal("got valid=true for a transition pointing at a state that doesn't exist")
	}
}

func TestValidateSpecLocally_ValidSpecPasses(t *testing.T) {
	spec := xclient.MachineSpec{
		Name:    "door",
		Initial: "closed",
		States:  map[string]xclient.StateDef{"closed": {}, "open": {}},
		Transitions: []xclient.TransitionDef{
			{From: rawFrom(t, "closed"), Input: "push", To: "open"},
			{From: rawFrom(t, "open"), Input: "close", To: "closed"},
		},
	}
	result := validateSpecLocally(spec)
	if !result.Valid {
		t.Fatalf("got valid=false for a well-formed spec, errors: %v", result.Errors)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v, want none for a valid spec", result.Errors)
	}
}

func TestValidateSpecLocally_WarnsOnUnreachableState(t *testing.T) {
	spec := xclient.MachineSpec{
		Name:    "m",
		Initial: "a",
		States:  map[string]xclient.StateDef{"a": {}, "b": {}, "orphan": {}},
		Transitions: []xclient.TransitionDef{
			{From: rawFrom(t, "a"), Input: "go", To: "b"},
		},
	}
	result := validateSpecLocally(spec)
	if !result.Valid {
		t.Fatalf("got valid=false, want true — an unreachable state is a warning, not a hard error: %v", result.Errors)
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "reachable") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %v, want one mentioning the unreachable state", result.Warnings)
	}
}

func TestValidateSpecLocally_MalformedSpecIsInvalidNotAPanic(t *testing.T) {
	spec := xclient.MachineSpec{
		Name:    "m",
		Initial: "a",
		States:  map[string]xclient.StateDef{"a": {}},
		Transitions: []xclient.TransitionDef{
			{From: json.RawMessage(`{"bad":true}`), Input: "go", To: "a"},
		},
	}
	result := validateSpecLocally(spec)
	if result.Valid {
		t.Fatal("got valid=true for a spec with a malformed transition From field")
	}
	if len(result.Errors) == 0 {
		t.Fatal("want at least one error message describing the malformed field")
	}
}
