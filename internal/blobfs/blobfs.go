// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package blobfs presents xolu's flat blob key store as a navigable
// folder hierarchy — a xoluman-only presentation convention, not
// anything xolu itself is aware of. xolu blob keys cannot contain "/"
// (confirmed enforced even on the S3-compatible surface, same
// validateKey as the native endpoint) — ":" is used instead, within an
// otherwise-flat key (photos:2026:vacation.jpg), translated to a
// folder path only at this package's boundary. The stored key is
// always one opaque flat string as far as xolu is concerned.
//
// Empty folders — which a blob-key scan alone can never reveal, since
// there is nothing to scan — are backed by a xoluman_blob_folder
// entity type, using xolu's own entity/REF mechanism (not a
// xoluman-local index), so they travel with the target instance's own
// backup/export and are visible to any other client of that instance.
// Created schema-lessly, same established pattern as
// internal/fieldmeta's xoluman_field_meta.
//
// The blob scan is always the source of truth for anything it can see
// (real files, and folders implied by them); the entity store only
// carries what the blob store structurally cannot: empty folders, and
// the "a person meant this to exist" bit (Explicit). Every browse
// reconciles the two: a blob-scan-implied folder with no matching
// entity gets one materialized now (Explicit: false); an implicit
// entity that no longer has any blobs or child folders backing it is
// removed, since it only ever existed as a side effect of content
// being there.
package blobfs

import (
	"context"
	"fmt"
	"strings"

	"github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/xoluext"
)

// FolderEntityType is the bookkeeping entity type name empty-folder
// records are stored under.
const FolderEntityType = "xoluman_blob_folder"

// folder is one xoluman_blob_folder entity, decoded from its raw
// document.
type folder struct {
	ID       int64
	Name     string
	ParentID int64 // 0 means root (no parent)
	Explicit bool
}

// Item is one entry shown when browsing a folder — a file (a real
// blob) or a subfolder (real, empty-but-explicit, or auto-materialized).
type Item struct {
	Name        string
	IsFolder    bool
	Size        int64  // files only
	ContentType string // files only
	SHA256      string // files only
	Explicit    bool   // folders only — whether a person deliberately created this empty folder
}

// KeyForPath joins path segments into the colon-delimited flat key
// xolu actually stores. Empty for the root.
func KeyForPath(path []string) string {
	return strings.Join(path, ":")
}

// prefixForPath is KeyForPath with a trailing delimiter, for BlobList's
// prefix matching — "photos:2026:", not "photos:2026", since a plain
// "photos:2026" would also match a sibling key like
// "photos:20260101notes.txt" the trailing colon is meant to exclude.
func prefixForPath(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return KeyForPath(path) + ":"
}

// List returns the items at path — the reconciled union of a real blob
// prefix scan and any matching xoluman_blob_folder entities, healing
// drift both directions along the way (see package doc).
func List(ctx context.Context, c *client.Client, path []string) ([]Item, error) {
	prefix := prefixForPath(path)

	blobResult, err := c.BlobList(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("listing blobs: %w", err)
	}

	// Partition the scan into direct files and implied subfolder names
	// (deduplicated — many files can share one subfolder).
	var files []Item
	impliedFolders := make(map[string]bool)
	for _, blob := range blobResult.Blobs {
		rest := strings.TrimPrefix(blob.Key, prefix)
		if rest == "" {
			continue // the prefix itself, if ever listed as its own key — shouldn't happen, defensive
		}
		if seg, _, isNested := strings.Cut(rest, ":"); isNested {
			impliedFolders[seg] = true
		} else {
			files = append(files, Item{Name: rest, Size: blob.Size, ContentType: blob.ContentType, SHA256: blob.SHA256})
		}
	}

	parentID, err := resolveFolderID(ctx, c, path)
	if err != nil {
		return nil, err
	}

	children, err := childFolders(ctx, c, parentID)
	if err != nil {
		return nil, fmt.Errorf("listing folder entities: %w", err)
	}
	childByName := make(map[string]folder, len(children))
	for _, f := range children {
		childByName[f.Name] = f
	}

	var folders []Item
	for name := range impliedFolders {
		if f, known := childByName[name]; known {
			folders = append(folders, Item{Name: name, IsFolder: true, Explicit: f.Explicit})
			delete(childByName, name) // handled — remaining entries are entity-only
			continue
		}
		// Blob-scan implies a folder with no matching entity yet —
		// materialize it now (Explicit: false).
		if _, err := createFolder(ctx, c, name, parentID, false); err != nil {
			return nil, fmt.Errorf("materializing folder %q: %w", name, err)
		}
		folders = append(folders, Item{Name: name, IsFolder: true, Explicit: false})
	}

	// Whatever's left in childByName is entity-only: not implied by the
	// current blob scan. Explicit ones persist regardless of contents.
	// Implicit ones only existed as a side effect of blobs that are no
	// longer there — garbage-collect them, but only if they also have
	// no child folders of their own (a still-populated implicit folder
	// two levels down must not be silently orphaned).
	for _, f := range childByName {
		if f.Explicit {
			folders = append(folders, Item{Name: f.Name, IsFolder: true, Explicit: true})
			continue
		}
		hasChildren, err := hasAnyChildFolder(ctx, c, f.ID)
		if err != nil {
			return nil, fmt.Errorf("checking children of folder %q before GC: %w", f.Name, err)
		}
		if hasChildren {
			folders = append(folders, Item{Name: f.Name, IsFolder: true, Explicit: false})
			continue
		}
		if err := deleteFolder(ctx, c, f.ID); err != nil {
			return nil, fmt.Errorf("garbage-collecting folder %q: %w", f.Name, err)
		}
		// Not added to the result — it's gone.
	}

	items := make([]Item, 0, len(folders)+len(files))
	items = append(items, folders...)
	items = append(items, files...)
	return items, nil
}

// DeleteExplicitFolder removes an explicit empty folder entity at
// parentPath/name. Does not itself verify the folder is actually
// empty — callers (the UI handler) that need that guarantee should
// call List on the folder's own path first and refuse to proceed if
// it returns anything; this function trusts the caller has already
// decided deletion is safe, since blobfs itself has no independent
// notion of "the click that triggered this is still valid" versus one
// based on a listing already re-checked a moment before.
func DeleteExplicitFolder(ctx context.Context, c *client.Client, parentPath []string, name string) error {
	parentID, err := resolveFolderID(ctx, c, parentPath)
	if err != nil {
		return err
	}
	children, err := childFolders(ctx, c, parentID)
	if err != nil {
		return fmt.Errorf("finding folder to delete: %w", err)
	}
	for _, f := range children {
		if f.Name == name {
			return deleteFolder(ctx, c, f.ID)
		}
	}
	return fmt.Errorf("blobfs: no folder named %q found to delete", name)
}

// CreateExplicitFolder creates an empty folder at path deliberately —
// Explicit: true, so it persists even if no blob is ever stored under
// it. Returns an error if a folder or file already occupies that name
// at that level (checked via a fresh List — see the comment inline).
func CreateExplicitFolder(ctx context.Context, c *client.Client, parentPath []string, name string) error {
	if strings.ContainsAny(name, ":/\\") {
		return fmt.Errorf("blobfs: folder name %q cannot contain \":\", \"/\", or \"\\\\\"", name)
	}
	parentID, err := resolveFolderID(ctx, c, parentPath)
	if err != nil {
		return err
	}
	existing, err := childFolders(ctx, c, parentID)
	if err != nil {
		return fmt.Errorf("checking for an existing folder: %w", err)
	}
	for _, f := range existing {
		if f.Name == name {
			return fmt.Errorf("blobfs: a folder named %q already exists here", name)
		}
	}
	_, err = createFolder(ctx, c, name, parentID, true)
	return err
}

// resolveFolderID walks path from the root, following parent-by-name
// lookups, and returns the deepest folder's entity ID — 0 for the
// root. Intermediate levels that exist only implicitly (blob-implied,
// never browsed into yet) are fine; this only needs IDs for entities
// that already exist, which List's own materialization keeps current
// for anything actually browsed.
func resolveFolderID(ctx context.Context, c *client.Client, path []string) (int64, error) {
	var parentID int64
	for _, segment := range path {
		children, err := childFolders(ctx, c, parentID)
		if err != nil {
			return 0, fmt.Errorf("resolving folder path: %w", err)
		}
		found := false
		for _, f := range children {
			if f.Name == segment {
				parentID = f.ID
				found = true
				break
			}
		}
		if !found {
			// No entity for this level yet — it exists only as far as
			// the blob scan is concerned. Materialize it so the rest
			// of the path (and anything the caller does next, like
			// creating a child folder under it) has a real parent to
			// attach to.
			id, err := createFolder(ctx, c, segment, parentID, false)
			if err != nil {
				return 0, fmt.Errorf("materializing intermediate folder %q: %w", segment, err)
			}
			parentID = id
		}
	}
	return parentID, nil
}

// childFolders returns every xoluman_blob_folder entity whose parent
// is parentID (0 meaning root — no parent field set at all).
func childFolders(ctx context.Context, c *client.Client, parentID int64) ([]folder, error) {
	entities, err := xoluext.ListAll(ctx, c, FolderEntityType)
	if err != nil {
		if xoluErr, ok := err.(*client.Error); ok && xoluErr.HTTPStatus == 404 {
			return nil, nil // entity type doesn't exist yet — no folders have ever been created
		}
		return nil, err
	}
	var out []folder
	for _, e := range entities {
		f := decodeFolder(e)
		if f.ParentID == parentID {
			out = append(out, f)
		}
	}
	return out, nil
}

// hasAnyChildFolder is a narrower existence check than childFolders,
// used only to decide whether an implicit, blob-less folder is safe to
// garbage-collect (it isn't, if something is still parented under it).
func hasAnyChildFolder(ctx context.Context, c *client.Client, parentID int64) (bool, error) {
	children, err := childFolders(ctx, c, parentID)
	if err != nil {
		return false, err
	}
	return len(children) > 0, nil
}

func decodeFolder(e client.Entity) folder {
	f := folder{ID: e.ID}
	if name, ok := e.Data["name"].(string); ok {
		f.Name = name
	}
	if explicit, ok := e.Data["explicit"].(bool); ok {
		f.Explicit = explicit
	}
	if parent, ok := e.Data["parent"].(float64); ok {
		f.ParentID = int64(parent)
	}
	return f
}

func createFolder(ctx context.Context, c *client.Client, name string, parentID int64, explicit bool) (int64, error) {
	data := map[string]any{"name": name, "explicit": explicit}
	if parentID != 0 {
		data["parent"] = parentID
	}
	entity, err := c.Create(ctx, FolderEntityType, data)
	if err != nil {
		return 0, err
	}
	return entity.ID, nil
}

func deleteFolder(ctx context.Context, c *client.Client, id int64) error {
	return c.Delete(ctx, FolderEntityType, id)
}
