// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package web

import (
	"io/fs"
	"testing"
)

// vendoredFiles must match VENDOR.md exactly — this test exists so a
// vendored file going missing (or VENDOR.md drifting from what's
// actually embedded) fails the build, rather than being caught only by
// someone happening to notice at runtime.
var vendoredFiles = []string{
	"static/vendor/htmx@1.9.10.min.js",
	"static/vendor/tabulator@6.5.2.min.js",
	"static/vendor/tabulator@6.5.2.min.css",
	"static/vendor/lit@3.js",
	"static/vendor/codemirror-bundle@1.js",
	"static/vendor/VENDOR.md",
}

func TestVendorFilesPresent(t *testing.T) {
	for _, path := range vendoredFiles {
		info, err := fs.Stat(Static, path)
		if err != nil {
			t.Errorf("%s: not present in the embedded FS: %v", path, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s: present but empty", path)
		}
	}
}

func TestVendorFilesPresent_CatchesAMissingFile(t *testing.T) {
	// Self-check on the check itself: confirms fs.Stat genuinely
	// returns an error for something absent, so a real regression in
	// TestVendorFilesPresent above isn't silently passing for the
	// wrong reason.
	if _, err := fs.Stat(Static, "static/vendor/does-not-exist.js"); err == nil {
		t.Fatal("fs.Stat on a nonexistent embedded path returned no error — the presence check above cannot be trusted")
	}
}

// ownAuthoredJSFiles are xoluman's own static JS, not third-party
// vendored code (VENDOR.md doesn't cover these) — but a page depending
// on one that's missing from the embed fails exactly the same way, so
// it gets the same presence check.
var ownAuthoredJSFiles = []string{
	"static/js/modal.js",
	"static/js/grid-editor.js",
	"static/js/query-editor.js",
	"static/js/dxp-editor.js",
	"static/js/fsm-editor.js",
}

func TestOwnAuthoredJSFilesPresent(t *testing.T) {
	for _, path := range ownAuthoredJSFiles {
		info, err := fs.Stat(Static, path)
		if err != nil {
			t.Errorf("%s: not present in the embedded FS: %v", path, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s: present but empty", path)
		}
	}
}

// ownAuthoredCSSFiles are xoluman's own static CSS, not vendored code
// or the Tailwind build output (which has no presence check of its own
// today, generated fresh by `make css` every time rather than being a
// static asset someone could accidentally leave out) — same reasoning
// as ownAuthoredJSFiles: a page depending on one that's missing from
// the embed fails silently otherwise.
var ownAuthoredCSSFiles = []string{
	"static/css/tabulator-dark-overrides.css",
}

func TestOwnAuthoredCSSFilesPresent(t *testing.T) {
	for _, path := range ownAuthoredCSSFiles {
		info, err := fs.Stat(Static, path)
		if err != nil {
			t.Errorf("%s: not present in the embedded FS: %v", path, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s: present but empty", path)
		}
	}
}
