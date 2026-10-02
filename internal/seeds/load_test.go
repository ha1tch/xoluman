// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package seeds

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSeedDir builds a minimal, real, on-disk seed directory whose
// manifest is exactly manifestJSON, plus every extra file listed in
// files (relative path -> content). Returns the directory's own path.
func writeSeedDir(t *testing.T, manifestJSON string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, manifestFileName), []byte(manifestJSON), 0644); err != nil {
		t.Fatalf("writing manifest: %v", err)
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
	return dir
}

func TestLoad_ValidSeedWithAllFilesPresent(t *testing.T) {
	dir := writeSeedDir(t,
		`{"format_version":1,"id":"x","name":"X","manual":"README.md","preview_images":["preview/a.png"],
		  "steps":[{"type":"fsm","file":"fsms/a.json"}]}`,
		map[string]string{
			"README.md":     "# X\n\nA test seed.",
			"preview/a.png": "fake-png-bytes",
			"fsms/a.json":   `{"name":"a"}`,
		},
	)
	ls, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ls.Manifest.ID != "x" {
		t.Errorf("ID = %q, want x", ls.Manifest.ID)
	}
	if ls.Dir != dir {
		t.Errorf("Dir = %q, want %q", ls.Dir, dir)
	}
}

func TestLoad_MissingManifestFile(t *testing.T) {
	dir := t.TempDir() // no seed.json written at all
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected an error for a missing manifest, got none")
	}
}

func TestLoad_InvalidManifestContent(t *testing.T) {
	dir := writeSeedDir(t, `{not valid json`, nil)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected an error for invalid manifest JSON, got none")
	}
}

func TestLoad_MissingStepFile(t *testing.T) {
	dir := writeSeedDir(t,
		`{"format_version":1,"id":"x","name":"X","steps":[{"type":"fsm","file":"fsms/missing.json"}]}`,
		nil, // fsms/missing.json deliberately never written
	)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected an error for a missing step file, got none")
	}
	if !strings.Contains(err.Error(), "fsms/missing.json") {
		t.Errorf("error %q does not name the missing file", err.Error())
	}
}

func TestLoad_MissingManual(t *testing.T) {
	dir := writeSeedDir(t,
		`{"format_version":1,"id":"x","name":"X","manual":"MISSING.md","steps":[{"type":"fsm","file":"fsms/a.json"}]}`,
		map[string]string{"fsms/a.json": `{}`},
	)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected an error for a missing manual file, got none")
	}
}

func TestLoad_MissingPreviewImage(t *testing.T) {
	dir := writeSeedDir(t,
		`{"format_version":1,"id":"x","name":"X","preview_images":["preview/missing.png"],"steps":[{"type":"fsm","file":"fsms/a.json"}]}`,
		map[string]string{"fsms/a.json": `{}`},
	)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected an error for a missing preview image, got none")
	}
}

func TestLoad_NoManualIsNotAnError(t *testing.T) {
	dir := writeSeedDir(t,
		`{"format_version":1,"id":"x","name":"X","steps":[{"type":"fsm","file":"fsms/a.json"}]}`,
		map[string]string{"fsms/a.json": `{}`},
	)
	ls, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	manual, err := ls.ReadManual()
	if err != nil {
		t.Fatalf("unexpected error reading absent manual: %v", err)
	}
	if manual != "" {
		t.Errorf("manual = %q, want empty string for a seed with no manual declared", manual)
	}
}

func TestLoad_ReadManual(t *testing.T) {
	dir := writeSeedDir(t,
		`{"format_version":1,"id":"x","name":"X","manual":"README.md","steps":[{"type":"fsm","file":"fsms/a.json"}]}`,
		map[string]string{"README.md": "# Hello", "fsms/a.json": `{}`},
	)
	ls, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	manual, err := ls.ReadManual()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if manual != "# Hello" {
		t.Errorf("manual = %q, want %q", manual, "# Hello")
	}
}

func TestLoad_ReadStepFile(t *testing.T) {
	dir := writeSeedDir(t,
		`{"format_version":1,"id":"x","name":"X","steps":[{"type":"fsm","file":"fsms/a.json"}]}`,
		map[string]string{"fsms/a.json": `{"name":"real content"}`},
	)
	ls, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := ls.ReadStepFile(ls.Manifest.Steps[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `{"name":"real content"}` {
		t.Errorf("ReadStepFile = %q, want the real file content", data)
	}
}
