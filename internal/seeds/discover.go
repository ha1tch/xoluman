// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package seeds

import (
	"os"
	"path/filepath"
)

// DiscoverFailure names one subdirectory that looked like a seed
// package (it exists, it's a directory) but failed to load, and why.
// Kept separate from a hard error: one malformed seed among several
// good ones should never hide the good ones from the browse list.
type DiscoverFailure struct {
	Dir string
	Err error
}

// DiscoverResult is what scanning a base directory of seed packages
// found: every seed that loaded successfully, plus every subdirectory
// that looked like a seed but didn't load, with why.
type DiscoverResult struct {
	Seeds    []*LoadedSeed
	Failures []DiscoverFailure
}

// Discover scans baseDir's immediate subdirectories for seed
// packages — any subdirectory containing a seed.json is attempted. A
// subdirectory without seed.json is silently not a seed at all (not a
// failure); a subdirectory with seed.json that fails to Load is a
// DiscoverFailure, not a hard error for the whole scan — the browse
// UI this exists for needs to show every seed that DID load even when
// one other seed package is broken.
//
// baseDir itself not existing is treated as "no seeds configured" —
// (nil, nil) — not an error: a fresh xoluman install with no local
// seeds directory set up yet is the normal case, not a fault.
func Discover(baseDir string) (*DiscoverResult, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &DiscoverResult{}, nil
		}
		return nil, err
	}

	result := &DiscoverResult{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(baseDir, entry.Name())
		manifestPath := filepath.Join(dir, manifestFileName)
		if _, err := os.Stat(manifestPath); err != nil {
			continue // not a seed directory at all, not a failure
		}
		ls, err := Load(dir)
		if err != nil {
			result.Failures = append(result.Failures, DiscoverFailure{Dir: dir, Err: err})
			continue
		}
		result.Seeds = append(result.Seeds, ls)
	}
	return result, nil
}
