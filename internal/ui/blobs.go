// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	mi "github.com/ha1tch/minty"
	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/blobfs"
	"github.com/ha1tch/xoluman/internal/connstore"
	"github.com/ha1tch/xoluman/internal/modules"
	"github.com/ha1tch/xoluman/internal/xoluext"
)

// RegisterBlobsModule registers the blob browser's routes onto reg,
// backed by store. No standalone nav entry (Label left empty, same
// reasoning as Entities) — blobs are reached per-connection.
//
// Only the browse view (List) sits on the {path...} wildcard route —
// Go's ServeMux wildcard is a full-suffix match, so a sibling route
// like ".../download" under the same wildcard tree is not expressible
// (it would just be absorbed as part of {path...} itself). Every other
// action lives on its own fixed route instead, with the target blob
// key or parent path carried as form data rather than URL path
// segments — avoids the collision entirely, at the cost of one query/
// form field per action instead of a path segment.
func RegisterBlobsModule(reg *modules.Registry, store connstore.Store) {
	h := &blobsHandler{store: store}
	reg.Register(modules.Module{
		ID: "blobs",
		MountRoutes: func(mux *http.ServeMux) {
			mux.HandleFunc("GET /connections/{name}/blobs", h.List)
			mux.HandleFunc("GET /connections/{name}/blobs/{path...}", h.List)
			mux.HandleFunc("POST /connections/{name}/blob-upload", h.Upload)
			mux.HandleFunc("GET /connections/{name}/blob-download", h.Download)
			mux.HandleFunc("POST /connections/{name}/blob-delete", h.DeleteFile)
			mux.HandleFunc("POST /connections/{name}/blob-new-folder", h.NewFolder)
			mux.HandleFunc("POST /connections/{name}/blob-delete-folder", h.DeleteFolder)
		},
	})
}

type blobsHandler struct {
	store connstore.Store
}

func (h *blobsHandler) clientFor(ctx context.Context, name string) (*xclient.Client, error) {
	conn, err := h.store.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	return xoluext.BuildClient(conn), nil
}

// splitPath turns the URL's slash-separated {path...} value into
// segments — blobfs's own vocabulary, which knows nothing about URLs
// and joins with ":" internally, never "/". Segments arrive already
// percent-decoded — Go's http server does this automatically for path
// wildcards — so this is a plain split, no unescaping needed here.
func splitPath(urlPath string) []string {
	urlPath = strings.Trim(urlPath, "/")
	if urlPath == "" {
		return nil
	}
	return strings.Split(urlPath, "/")
}

// joinPath is for hidden form field VALUES only (the "path" input on
// the upload/new-folder/delete-folder forms) — deliberately NOT
// percent-escaped. A browser's own form submission already percent-
// encodes an attribute's raw text for the form-urlencoded body, and Go
// decodes it back automatically on the receiving end; escaping it here
// too would double-encode and corrupt the round trip through
// splitPath. Never use this to build an href — see joinPathForURL.
func joinPath(segments []string) string {
	return strings.Join(segments, "/")
}

// joinPathForURL is for actual href/action URLs — segments are
// user-controlled (a folder name someone typed, or a filename someone
// uploaded), and unlike a form submission, a browser does not
// re-encode an href's literal text before navigating to it. A literal
// "?", "#", or space in a folder name would otherwise be
// misinterpreted as URL structure (a query string, a fragment) rather
// than a literal character in the path — real bug, same class as the
// connection-name one fixed elsewhere in this codebase, confirmed by
// the same kind of direct test before this existed.
func joinPathForURL(segments []string) string {
	escaped := make([]string, len(segments))
	for i, s := range segments {
		escaped[i] = url.PathEscape(s)
	}
	return strings.Join(escaped, "/")
}

func (h *blobsHandler) List(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	path := splitPath(r.PathValue("path"))

	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	items, err := blobfs.List(r.Context(), c, path)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	body := blobBrowserBody(name, path, items)
	WriteHTML(w, Page("Blobs — "+name, r.URL.Path, body))
}

func blobBrowserBody(connName string, path []string, items []blobfs.Item) mi.H {
	return func(b *mi.Builder) mi.Node {
		basePath := "/connections/" + url.PathEscape(connName) + "/blobs"

		// Breadcrumb: "root" plus one link per path segment, each
		// pointing at the URL for browsing up to that level.
		crumbs := []interface{}{b.A(mi.Href(basePath), mi.Class("text-indigo-600 dark:text-indigo-400 hover:underline"), "root")}
		for i, seg := range path {
			crumbs = append(crumbs, " / ")
			crumbURL := basePath + "/" + joinPathForURL(path[:i+1])
			crumbs = append(crumbs, b.A(mi.Href(crumbURL), mi.Class("text-indigo-600 dark:text-indigo-400 hover:underline"), seg))
		}
		breadcrumb := b.Div(append([]interface{}{mi.Class("text-sm mb-4")}, crumbs...)...)

		td := "px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-sm"
		rows := make([]mi.Node, len(items))
		for i, item := range items {
			if item.IsFolder {
				folderURL := basePath + "/" + joinPathForURL(append(append([]string{}, path...), item.Name))
				explicitLabel := ""
				if item.Explicit {
					explicitLabel = " (kept empty)"
				}
				var deleteAction interface{}
				if item.Explicit {
					deleteAction = deleteFolderForm(connName, path, item.Name)(b)
				}
				rows[i] = b.Tr(
					b.Td(mi.Class(td), b.A(mi.Href(folderURL), mi.Class("text-indigo-600 dark:text-indigo-400 hover:underline font-medium"), "📁 "+item.Name+explicitLabel)),
					b.Td(mi.Class(td), ""),
					b.Td(mi.Class(td), deleteAction),
				)
				continue
			}
			key := blobfs.KeyForPath(append(append([]string{}, path...), item.Name))
			downloadURL := "/connections/" + url.PathEscape(connName) + "/blob-download?key=" + urlQueryEscape(key)
			rows[i] = b.Tr(
				b.Td(mi.Class(td), b.A(mi.Href(downloadURL), mi.Class("text-indigo-600 dark:text-indigo-400 hover:underline"), item.Name)),
				b.Td(mi.Class(td), formatBytes(item.Size)),
				b.Td(mi.Class(td), deleteFileForm(connName, key)(b)),
			)
		}
		table := Table([]string{"Name", "Size", ""}, rows, "This folder is empty.")(b)

		uploadForm := b.Form(mi.Attr("method", "post"), mi.Attr("action", "/connections/"+url.PathEscape(connName)+"/blob-upload"), mi.Attr("enctype", "multipart/form-data"), mi.Class("flex gap-2 items-end mb-3"),
			b.Input(mi.Type("hidden"), mi.Name("path"), mi.Value(joinPath(path))),
			b.Input(mi.Type("file"), mi.Name("file"), mi.Required()),
			b.Button(mi.Type("submit"), mi.Class(btnSecondary), "Upload"),
		)
		newFolderForm := b.Form(mi.Attr("method", "post"), mi.Attr("action", "/connections/"+url.PathEscape(connName)+"/blob-new-folder"), mi.Class("flex gap-2 items-end mb-4"),
			b.Input(mi.Type("hidden"), mi.Name("path"), mi.Value(joinPath(path))),
			b.Input(mi.Type("text"), mi.Name("name"), mi.Placeholder("new folder name"), mi.Class(formInputClass), mi.Required()),
			b.Button(mi.Type("submit"), mi.Class(btnSecondary), "New folder"),
		)

		return b.Div(
			b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white mb-2"), "Blobs on "+connName),
			breadcrumb,
			uploadForm,
			newFolderForm,
			table,
		)
	}
}

func deleteFileForm(connName, key string) mi.H {
	return func(b *mi.Builder) mi.Node {
		return b.Form(mi.Attr("method", "post"), mi.Attr("action", "/connections/"+url.PathEscape(connName)+"/blob-delete"), mi.Style("display:inline;"),
			b.Input(mi.Type("hidden"), mi.Name("key"), mi.Value(key)),
			b.Button(mi.Type("submit"), mi.Class(btnDanger), "Delete"),
		)
	}
}

func deleteFolderForm(connName string, parentPath []string, folderName string) mi.H {
	return func(b *mi.Builder) mi.Node {
		return b.Form(mi.Attr("method", "post"), mi.Attr("action", "/connections/"+url.PathEscape(connName)+"/blob-delete-folder"), mi.Style("display:inline;"),
			b.Input(mi.Type("hidden"), mi.Name("path"), mi.Value(joinPath(parentPath))),
			b.Input(mi.Type("hidden"), mi.Name("name"), mi.Value(folderName)),
			b.Button(mi.Type("submit"), mi.Class(btnDanger), "Delete"),
		)
	}
}

// Upload streams an uploaded file straight through to BlobPut — no
// server-side temp file, matching the same streaming discipline as
// BlobGet/Download.
func (h *blobsHandler) Upload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	if err := r.ParseMultipartForm(20 << 20); err != nil {
		http.Error(w, "parsing upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	path := splitPath(r.FormValue("path"))

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "reading uploaded file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()

	key := blobfs.KeyForPath(append(append([]string{}, path...), header.Filename))
	contentType := header.Header.Get("Content-Type")
	if _, err := c.BlobPut(r.Context(), key, contentType, file); err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	http.Redirect(w, r, blobBrowseURL(name, path), http.StatusSeeOther)
}

// Download streams a blob's content straight through from BlobGet to
// the response — never buffered, per BlobGet's own streaming contract.
func (h *blobsHandler) Download(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}

	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	result, err := c.BlobGet(r.Context(), key)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}
	defer func() { _ = result.Body.Close() }()

	filename := key
	if idx := strings.LastIndex(key, ":"); idx >= 0 {
		filename = key[idx+1:]
	}
	if result.ContentType != "" {
		w.Header().Set("Content-Type", result.ContentType)
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if result.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(result.Size, 10))
	}
	_, _ = io.Copy(w, result.Body)
}

func (h *blobsHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "parsing form: "+err.Error(), http.StatusBadRequest)
		return
	}
	key := r.PostForm.Get("key")
	if key == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}

	if _, err := c.BlobDelete(r.Context(), key); err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	parent := splitPath(key)
	if len(parent) > 0 {
		parent = parent[:len(parent)-1]
	}
	http.Redirect(w, r, blobBrowseURL(name, parent), http.StatusSeeOther)
}

func (h *blobsHandler) NewFolder(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "parsing form: "+err.Error(), http.StatusBadRequest)
		return
	}
	path := splitPath(r.PostForm.Get("path"))
	folderName := r.PostForm.Get("name")

	if err := blobfs.CreateExplicitFolder(r.Context(), c, path, folderName); err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	http.Redirect(w, r, blobBrowseURL(name, path), http.StatusSeeOther)
}

// DeleteFolder removes an explicit empty folder — refuses if it isn't
// actually empty (has files or subfolders), by re-listing it first
// rather than trusting the click alone; the listing the person is
// looking at could be stale by the time they click Delete.
func (h *blobsHandler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c, err := h.clientFor(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "parsing form: "+err.Error(), http.StatusBadRequest)
		return
	}
	parentPath := splitPath(r.PostForm.Get("path"))
	folderName := r.PostForm.Get("name")
	folderPath := append(append([]string{}, parentPath...), folderName)

	contents, err := blobfs.List(r.Context(), c, folderPath)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}
	if len(contents) > 0 {
		http.Error(w, "folder is not empty", http.StatusConflict)
		return
	}

	if err := blobfs.DeleteExplicitFolder(r.Context(), c, parentPath, folderName); err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	http.Redirect(w, r, blobBrowseURL(name, parentPath), http.StatusSeeOther)
}

func blobBrowseURL(connName string, path []string) string {
	base := "/connections/" + url.PathEscape(connName) + "/blobs"
	if len(path) == 0 {
		return base
	}
	return base + "/" + joinPathForURL(path)
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func urlQueryEscape(s string) string {
	return url.QueryEscape(s)
}
