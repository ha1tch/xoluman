// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package blobfs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ha1tch/xolu/pkg/client"
)

func TestKeyForPath(t *testing.T) {
	cases := []struct {
		path []string
		want string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"photos"}, "photos"},
		{[]string{"photos", "2026", "vacation.jpg"}, "photos:2026:vacation.jpg"},
	}
	for _, c := range cases {
		if got := KeyForPath(c.path); got != c.want {
			t.Errorf("KeyForPath(%v) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestPrefixForPath(t *testing.T) {
	if got := prefixForPath(nil); got != "" {
		t.Errorf("prefixForPath(nil) = %q, want empty", got)
	}
	if got := prefixForPath([]string{"photos", "2026"}); got != "photos:2026:" {
		t.Errorf("prefixForPath = %q, want %q", got, "photos:2026:")
	}
}

func TestList_EmptyRoot(t *testing.T) {
	f := newFakeXolu(t)
	items, err := List(context.Background(), f.client(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %v, want empty", items)
	}
}

func TestList_FlatFilesAtRoot(t *testing.T) {
	f := newFakeXolu(t)
	f.seedBlobs("readme.txt", "logo.png")

	items, err := List(context.Background(), f.client(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2: %+v", len(items), items)
	}
	for _, it := range items {
		if it.IsFolder {
			t.Fatalf("item %+v is a folder, want a file", it)
		}
	}
}

func TestList_ImpliedFolderFromNestedBlob(t *testing.T) {
	f := newFakeXolu(t)
	f.seedBlobs("photos:vacation.jpg")

	items, err := List(context.Background(), f.client(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || !items[0].IsFolder || items[0].Name != "photos" {
		t.Fatalf("items = %+v, want exactly one folder named \"photos\"", items)
	}
}

func TestList_MultipleFilesSameSubfolderDeduplicated(t *testing.T) {
	f := newFakeXolu(t)
	f.seedBlobs("photos:a.jpg", "photos:b.jpg", "photos:c.jpg")

	items, err := List(context.Background(), f.client(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want exactly 1 deduplicated \"photos\" folder: %+v", len(items), items)
	}
}

func TestList_BrowsingIntoSubfolderShowsItsFiles(t *testing.T) {
	f := newFakeXolu(t)
	f.seedBlobs("photos:vacation.jpg", "readme.txt")

	items, err := List(context.Background(), f.client(), []string{"photos"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].IsFolder || items[0].Name != "vacation.jpg" {
		t.Fatalf("items = %+v, want exactly the one file inside photos", items)
	}
}

func TestList_MaterializesImplicitFolderEntity(t *testing.T) {
	f := newFakeXolu(t)
	f.seedBlobs("photos:vacation.jpg")

	if _, err := List(context.Background(), f.client(), nil); err != nil {
		t.Fatalf("List: %v", err)
	}

	doc, found := f.folderNamed("photos")
	if !found {
		t.Fatal("browsing the root did not materialize a folder entity for \"photos\"")
	}
	if doc["explicit"] != false {
		t.Fatalf("materialized folder explicit = %v, want false", doc["explicit"])
	}
}

func TestList_SecondBrowseReusesMaterializedEntityNotDuplicating(t *testing.T) {
	f := newFakeXolu(t)
	f.seedBlobs("photos:vacation.jpg")

	if _, err := List(context.Background(), f.client(), nil); err != nil {
		t.Fatalf("first List: %v", err)
	}
	if _, err := List(context.Background(), f.client(), nil); err != nil {
		t.Fatalf("second List: %v", err)
	}

	if f.folderCount() != 1 {
		t.Fatalf("folder entity count = %d, want exactly 1 (no duplicate materialization)", f.folderCount())
	}
}

func TestCreateExplicitFolder_ShowsUpWithZeroBlobs(t *testing.T) {
	f := newFakeXolu(t)
	ctx := context.Background()

	if err := CreateExplicitFolder(ctx, f.client(), nil, "empty-folder"); err != nil {
		t.Fatalf("CreateExplicitFolder: %v", err)
	}

	items, err := List(ctx, f.client(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || !items[0].IsFolder || items[0].Name != "empty-folder" || !items[0].Explicit {
		t.Fatalf("items = %+v, want one explicit empty folder", items)
	}
}

func TestCreateExplicitFolder_DuplicateNameErrors(t *testing.T) {
	f := newFakeXolu(t)
	ctx := context.Background()

	if err := CreateExplicitFolder(ctx, f.client(), nil, "docs"); err != nil {
		t.Fatalf("first CreateExplicitFolder: %v", err)
	}
	if err := CreateExplicitFolder(ctx, f.client(), nil, "docs"); err == nil {
		t.Fatal("second CreateExplicitFolder with the same name: want an error")
	}
}

func TestCreateExplicitFolder_RejectsReservedCharacters(t *testing.T) {
	f := newFakeXolu(t)
	for _, bad := range []string{"a:b", "a/b", `a\b`} {
		if err := CreateExplicitFolder(context.Background(), f.client(), nil, bad); err == nil {
			t.Fatalf("CreateExplicitFolder(%q): want an error for a reserved character", bad)
		}
	}
}

func TestList_ExplicitFolderSurvivesWithNoBlobs(t *testing.T) {
	f := newFakeXolu(t)
	ctx := context.Background()

	if err := CreateExplicitFolder(ctx, f.client(), nil, "keep-me"); err != nil {
		t.Fatalf("CreateExplicitFolder: %v", err)
	}
	// Browse the root twice — an explicit folder must never be
	// garbage-collected regardless of blob content.
	if _, err := List(ctx, f.client(), nil); err != nil {
		t.Fatalf("first List: %v", err)
	}
	items, err := List(ctx, f.client(), nil)
	if err != nil {
		t.Fatalf("second List: %v", err)
	}
	if len(items) != 1 || items[0].Name != "keep-me" {
		t.Fatalf("items = %+v, want the explicit folder still present", items)
	}
}

func TestList_ImplicitFolderGarbageCollectedWhenBlobsGone(t *testing.T) {
	f := newFakeXolu(t)
	ctx := context.Background()

	f.seedBlobs("temp:file.txt")
	if _, err := List(ctx, f.client(), nil); err != nil {
		t.Fatalf("first List (materializes \"temp\"): %v", err)
	}
	if _, found := f.folderNamed("temp"); !found {
		t.Fatal("setup failed: \"temp\" wasn't materialized")
	}

	// The blob is gone now (nothing seeded means BlobList returns
	// nothing under "temp:" any more) — the next browse of the root
	// should GC the now-empty implicit folder.
	f.blobKeys = nil

	items, err := List(ctx, f.client(), nil)
	if err != nil {
		t.Fatalf("second List: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %+v, want the implicit folder gone", items)
	}
	if _, found := f.folderNamed("temp"); found {
		t.Fatal("\"temp\" folder entity still present, want it garbage-collected")
	}
}

func TestList_ImplicitFolderNotGCdIfItHasChildFolders(t *testing.T) {
	f := newFakeXolu(t)
	ctx := context.Background()

	// "archive" has no direct blobs of its own, but has a child folder
	// ("archive:2025") that does — materialize both by browsing deep,
	// then remove the leaf blob and confirm "archive" survives because
	// "archive:2025" (now itself empty-but-just-materialized) still
	// exists as an entity.
	f.seedBlobs("archive:2025:old.txt")
	if _, err := List(ctx, f.client(), nil); err != nil {
		t.Fatalf("List root: %v", err)
	}
	if _, err := List(ctx, f.client(), []string{"archive"}); err != nil {
		t.Fatalf("List archive: %v", err)
	}

	// Now remove the underlying blob entirely and re-browse root. The
	// nested "archive:2025" is itself implicit-and-now-blobless, so
	// walking root->archive->2025 in one pass triggers GC bottom is
	// out of scope for this single call; what this test actually
	// checks is that "archive" (which has a child folder entity,
	// "archive:2025") is NOT removed merely because it directly has no
	// blobs of its own.
	f.blobKeys = nil

	items, err := List(ctx, f.client(), nil)
	if err != nil {
		t.Fatalf("List root after blob removal: %v", err)
	}
	if len(items) != 1 || items[0].Name != "archive" {
		t.Fatalf("items = %+v, want \"archive\" to survive because it still has a child folder entity", items)
	}
}

func TestResolveFolderID_NestedPath(t *testing.T) {
	f := newFakeXolu(t)
	ctx := context.Background()
	f.seedBlobs("a:b:c.txt")

	// Materialize the whole chain by browsing into the deepest level.
	if _, err := List(ctx, f.client(), []string{"a", "b"}); err != nil {
		t.Fatalf("List a/b: %v", err)
	}

	id, err := resolveFolderID(ctx, f.client(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("resolveFolderID: %v", err)
	}
	if id == 0 {
		t.Fatal("resolveFolderID returned 0 for a real nested path")
	}
	if _, found := f.folderNamed("b"); !found {
		t.Fatal("\"b\" folder entity not found — the nested path wasn't actually materialized")
	}
}

func TestResolveFolderID_RootIsZero(t *testing.T) {
	f := newFakeXolu(t)
	id, err := resolveFolderID(context.Background(), f.client(), nil)
	if err != nil {
		t.Fatalf("resolveFolderID: %v", err)
	}
	if id != 0 {
		t.Fatalf("resolveFolderID(root) = %d, want 0", id)
	}
}

func TestList_BlobListFailurePropagates(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/blob", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	_, err := List(context.Background(), client.New(server.URL), nil)
	if err == nil {
		t.Fatal("List: want an error when the blob scan itself fails")
	}
}

func TestList_FolderEntityListFailurePropagates(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/blob", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"tenant": "default", "count": 0, "blobs": []any{}})
	})
	mux.HandleFunc("GET /api/v1/xoluman_blob_folder", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	_, err := List(context.Background(), client.New(server.URL), nil)
	if err == nil {
		t.Fatal("List: want an error when the folder-entity fetch itself fails (not just 404)")
	}
}

func TestCreateExplicitFolder_ChildFoldersFetchFailurePropagates(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/xoluman_blob_folder", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	err := CreateExplicitFolder(context.Background(), client.New(server.URL), nil, "docs")
	if err == nil {
		t.Fatal("CreateExplicitFolder: want an error when checking for an existing folder fails")
	}
}
