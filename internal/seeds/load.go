// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package seeds

import (
	"fmt"
	"os"
	"path/filepath"
)

// manifestFileName is the manifest's own fixed filename within a seed
// directory — not configurable, so Discover can recognize a seed
// directory by this file's presence alone.
const manifestFileName = "seed.json"

// LoadedSeed is one seed package's manifest plus the real, absolute
// directory it was loaded from — every Step.File, Manifest.Manual,
// and Manifest.PreviewImages path is relative to Dir.
type LoadedSeed struct {
	Manifest *Manifest
	Dir      string
}

// Load reads dir/seed.json, parses and structurally validates it (see
// ParseManifest), and confirms every file the manifest references —
// the manual, every preview image, every step's own file — actually
// exists on disk. A seed that references a missing file is rejected
// here, at load time, rather than discovered later as a confusing
// mid-apply failure partway through a run.
func Load(dir string) (*LoadedSeed, error) {
	manifestPath := filepath.Join(dir, manifestFileName)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("seeds: reading %s: %w", manifestPath, err)
	}
	m, err := ParseManifest(data)
	if err != nil {
		return nil, err
	}

	checkExists := func(relPath, label string) error {
		full := filepath.Join(dir, relPath)
		if _, err := os.Stat(full); err != nil {
			return fmt.Errorf("seeds: manifest %q: %s %q: %w", m.ID, label, relPath, err)
		}
		return nil
	}

	if m.Manual != "" {
		if err := checkExists(m.Manual, "manual"); err != nil {
			return nil, err
		}
	}
	for _, img := range m.PreviewImages {
		if err := checkExists(img, "preview image"); err != nil {
			return nil, err
		}
	}
	for i, s := range m.Steps {
		if err := checkExists(s.File, fmt.Sprintf("step %d file", i)); err != nil {
			return nil, err
		}
	}

	return &LoadedSeed{Manifest: m, Dir: dir}, nil
}

// ReadStepFile reads one step's own file relative to the seed's own
// directory.
func (ls *LoadedSeed) ReadStepFile(s Step) ([]byte, error) {
	full := filepath.Join(ls.Dir, s.File)
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, fmt.Errorf("seeds: reading %s: %w", full, err)
	}
	return data, nil
}

// ReadManual reads the seed's own manual file. Returns ("", nil) when
// no manual is declared — an absent manual is not an error, just
// nothing to show in preview.
func (ls *LoadedSeed) ReadManual() (string, error) {
	if ls.Manifest.Manual == "" {
		return "", nil
	}
	full := filepath.Join(ls.Dir, ls.Manifest.Manual)
	data, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("seeds: reading manual %s: %w", full, err)
	}
	return string(data), nil
}
