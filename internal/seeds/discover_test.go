// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package seeds

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover_MissingBaseDirIsNotAnError(t *testing.T) {
	result, err := Discover(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Seeds) != 0 || len(result.Failures) != 0 {
		t.Fatalf("got %+v, want an empty result for a missing base dir", result)
	}
}

func TestDiscover_EmptyBaseDir(t *testing.T) {
	result, err := Discover(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Seeds) != 0 || len(result.Failures) != 0 {
		t.Fatalf("got %+v, want empty for an empty base dir", result)
	}
}

func TestDiscover_MultipleGoodSeeds(t *testing.T) {
	base := t.TempDir()
	writeSeedInBase(t, base, "seed-a", `{"format_version":1,"id":"a","name":"A","steps":[{"type":"fsm","file":"f.json"}]}`, map[string]string{"f.json": "{}"})
	writeSeedInBase(t, base, "seed-b", `{"format_version":1,"id":"b","name":"B","steps":[{"type":"fsm","file":"f.json"}]}`, map[string]string{"f.json": "{}"})

	result, err := Discover(base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Seeds) != 2 {
		t.Fatalf("got %d seeds, want 2", len(result.Seeds))
	}
	if len(result.Failures) != 0 {
		t.Fatalf("got %d failures, want 0: %+v", len(result.Failures), result.Failures)
	}
	ids := map[string]bool{}
	for _, s := range result.Seeds {
		ids[s.Manifest.ID] = true
	}
	if !ids["a"] || !ids["b"] {
		t.Fatalf("got ids %v, want both a and b", ids)
	}
}

func TestDiscover_OneBrokenSeedDoesNotHideGoodOnes(t *testing.T) {
	base := t.TempDir()
	writeSeedInBase(t, base, "seed-good", `{"format_version":1,"id":"good","name":"Good","steps":[{"type":"fsm","file":"f.json"}]}`, map[string]string{"f.json": "{}"})
	writeSeedInBase(t, base, "seed-broken", `{not valid json`, nil)

	result, err := Discover(base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Seeds) != 1 || result.Seeds[0].Manifest.ID != "good" {
		t.Fatalf("got %+v, want exactly the good seed", result.Seeds)
	}
	if len(result.Failures) != 1 {
		t.Fatalf("got %d failures, want 1", len(result.Failures))
	}
}

func TestDiscover_NonSeedDirectoryIsSkippedSilently(t *testing.T) {
	base := t.TempDir()
	// A plain subdirectory with no seed.json at all -- not a seed,
	// not a failure, just irrelevant to this scan.
	if err := os.MkdirAll(filepath.Join(base, "not-a-seed"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "not-a-seed", "readme.txt"), []byte("hi"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	result, err := Discover(base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Seeds) != 0 || len(result.Failures) != 0 {
		t.Fatalf("got %+v, want a non-seed directory to be silently skipped", result)
	}
}

func writeSeedInBase(t *testing.T, base, name, manifestJSON string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestFileName), []byte(manifestJSON), 0644); err != nil {
		t.Fatalf("writing manifest for %s: %v", name, err)
	}
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("writing %s: %v", rel, err)
		}
	}
}
