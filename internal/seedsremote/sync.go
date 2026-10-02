// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package seedsremote fetches the opt-in remote seed source
// (ha1tch/xoluseeds) into a local cache directory, from which it's
// then discoverable through exactly the same internal/seeds.Discover
// path as any local seed — no separate "remote seed" data model
// exists anywhere else in xoluman.
//
// Design (agreed): the whole remote fetch is one check, triggered
// once per explicit request (a Browse page load, or a manual retry
// click) — never a background poll, never an automatic retry on
// failure. A single .zip download (codeload.github.com, one request
// regardless of how many seed packages the repo holds) rather than
// the GitHub REST API's own per-directory contents calls, which would
// burn through its unauthenticated rate limit (60/hour) after a
// handful of seeds and, worse, turn "one check" into an unbounded
// number of requests as the repo grows. .zip rather than .tar.gz —
// codeload serves both as an equally single request, so the
// rate-limit argument doesn't favor either one; .zip's own simpler,
// non-nested container (no separate compression layer wrapping the
// archive structure, unlike gzip-wrapping-tar) was preferred with no
// good reason to keep tar.gz once that was actually checked, not just
// defended reflexively.
package seedsremote

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DefaultZipURL is ha1tch/xoluseeds' own zip archive on its default
// branch — confirmed directly (the repo's own default branch is
// "main", not "master"), not assumed.
const DefaultZipURL = "https://codeload.github.com/ha1tch/xoluseeds/zip/refs/heads/main"

// Sync fetches the zip archive at url and extracts it into destDir,
// replacing any prior content there entirely — destDir is a cache,
// not something to accumulate stale entries in across syncs. An empty
// repository (today's real state of ha1tch/xoluseeds) is not an
// error: the archive still downloads and extracts fine, it simply
// contains no seed subdirectories, which Discover already treats as
// "nothing found," not a failure.
//
// The whole response is read into memory before extraction —
// archive/zip needs random access (its central directory sits at the
// end of the file), unlike a tar stream, which could be processed
// entry-by-entry directly off the response body. A real cost only for
// archives too large to comfortably buffer; seed packages (schemas,
// JSONL data, small preview images) are not that.
func Sync(ctx context.Context, url, destDir string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("seedsremote: building request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("seedsremote: fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("seedsremote: fetching %s: HTTP %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("seedsremote: reading response body: %w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("seedsremote: opening zip archive: %w", err)
	}

	// Clear the cache before extracting -- a sync that partially fails
	// partway through must never leave a mix of this sync's files and
	// a prior sync's stale ones; better an empty or incomplete cache
	// (visibly "no seeds" or "some seeds") than a silently
	// inconsistent one.
	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("seedsremote: clearing cache dir: %w", err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("seedsremote: creating cache dir: %w", err)
	}

	cleanDest := filepath.Clean(destDir)
	for _, f := range zr.File {
		// GitHub's own zip wraps everything in a single top-level
		// "{owner}-{repo}-{sha}/" directory -- strip exactly that one
		// path component so extracted content lands directly as seed
		// subdirectories in destDir, matching what Discover expects
		// (destDir/{seed-id}/seed.json), not
		// destDir/{wrapper}/{seed-id}/seed.json.
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) < 2 || parts[1] == "" {
			continue // the wrapper directory entry itself
		}
		relPath := parts[1]

		// Path-traversal safety: reject any entry whose resolved
		// target would land outside destDir. A zip archive is
		// untrusted input the moment it comes from a network fetch,
		// same "zip slip"-class defense as the prior tar.gz
		// implementation had — checked even though this repository is
		// under known control today, since the defense costs nothing
		// and the failure mode (an entry escaping the cache directory)
		// is worth never risking.
		target := filepath.Join(destDir, relPath)
		if !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) && target != cleanDest {
			return fmt.Errorf("seedsremote: zip entry %q would escape the cache directory, refusing to extract", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("seedsremote: creating %s: %w", target, err)
			}
			continue
		}
		if !f.Mode().IsRegular() {
			// Symlinks and anything else: skipped, not erred — a seed
			// package has no legitimate use for them, and silently
			// ignoring is safer than either extracting an unvetted
			// symlink target or aborting the whole sync over an entry
			// no real seed package would ever contain.
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("seedsremote: creating %s: %w", filepath.Dir(target), err)
		}
		src, err := f.Open()
		if err != nil {
			return fmt.Errorf("seedsremote: opening zip entry %s: %w", f.Name, err)
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			src.Close()
			return fmt.Errorf("seedsremote: creating %s: %w", target, err)
		}
		_, copyErr := io.Copy(dst, src)
		src.Close()
		closeErr := dst.Close()
		if copyErr != nil {
			return fmt.Errorf("seedsremote: writing %s: %w", target, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("seedsremote: closing %s: %w", target, closeErr)
		}
	}
	return nil
}
