// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ha1tch/xoluman/internal/connstore"
)

// fakeXoluBlobs is a minimal, genuinely stateful blob+folder-entity
// fake for these handler tests — same reasoning as
// internal/blobfs/fake_xolu_test.go: the handlers' behavior depends on
// real interaction between the blob store and the folder-entity store,
// which a call-by-call mock can't represent honestly. A separate copy
// rather than reusing blobfs's internal one since that one is
// unexported to its own package.
type fakeXoluBlobs struct {
	mu      sync.Mutex
	blobs   map[string][]byte
	folders map[int64]map[string]any
	nextID  int64
}

func newFakeXoluBlobs(t *testing.T) *httptest.Server {
	t.Helper()
	f := &fakeXoluBlobs{blobs: map[string][]byte{}, folders: map[int64]map[string]any{}}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/blob", func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Query().Get("prefix")
		f.mu.Lock()
		defer f.mu.Unlock()
		var blobs []map[string]any
		for k, v := range f.blobs {
			if strings.HasPrefix(k, prefix) {
				blobs = append(blobs, map[string]any{"key": k, "sha256": "x", "size": float64(len(v)), "content_type": "application/octet-stream", "stored_at": "2026-08-04T00:00:00Z"})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"tenant": "default", "prefix": prefix, "count": len(blobs), "blobs": blobs})
	})

	mux.HandleFunc("POST /api/v1/blob", func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Blob-Key")
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.blobs[key] = body
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"key": key, "sha256": "x"})
	})

	mux.HandleFunc("GET /api/v1/blob/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		f.mu.Lock()
		body, ok := f.blobs[key]
		f.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-BL001", "message": "not found"}})
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	})

	mux.HandleFunc("DELETE /api/v1/blob/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		f.mu.Lock()
		_, ok := f.blobs[key]
		delete(f.blobs, key)
		f.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "XOLU-BL001", "message": "not found"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"key": key, "deleted": true})
	})

	mux.HandleFunc("GET /api/v1/xoluman_blob_folder", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		data := make([]map[string]any, 0, len(f.folders))
		for id, doc := range f.folders {
			row := map[string]any{"id": float64(id)}
			for k, v := range doc {
				row[k] = v
			}
			data = append(data, row)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "pagination": map[string]any{"page": 1, "per_page": len(data) + 1, "total_items": len(data), "total_pages": 1}})
	})

	mux.HandleFunc("POST /api/v1/xoluman_blob_folder", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.nextID++
		id := f.nextID
		f.folders[id] = body
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(id), "message": "created"})
	})

	mux.HandleFunc("DELETE /api/v1/xoluman_blob_folder/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		f.mu.Lock()
		delete(f.folders, id)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestJoinPathForURL_EscapesSegments(t *testing.T) {
	got := joinPathForURL([]string{"my folder", "weird?name"})
	want := "my%20folder/weird%3Fname"
	if got != want {
		t.Fatalf("joinPathForURL = %q, want %q", got, want)
	}
}

func TestJoinPath_DoesNotEscape(t *testing.T) {
	// Deliberately unescaped — see joinPath's own doc comment. Form
	// field values round-trip through the browser's own submission
	// encoding; pre-escaping here would double-encode.
	got := joinPath([]string{"my folder", "sub"})
	want := "my folder/sub"
	if got != want {
		t.Fatalf("joinPath = %q, want %q (unescaped)", got, want)
	}
}

func TestBlobsHandler_List_ConnectionNameWithSpaceEscapedInLinks(t *testing.T) {
	store := connstore.NewFileBackend(t.TempDir())
	server := newFakeXoluBlobs(t)
	if _, err := store.Save(context.Background(), connstore.Connection{
		ConnectionMeta: connstore.ConnectionMeta{Name: "My Server", BaseURL: server.URL, AuthMode: connstore.AuthNone},
	}); err != nil {
		t.Fatalf("seeding connection: %v", err)
	}
	h := &blobsHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/My%20Server/blobs", nil)
	req.SetPathValue("name", "My Server")
	rec := httptest.NewRecorder()
	h.List(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, `"/connections/My Server/`) {
		t.Fatalf("body contains an unescaped space in a connection-name-derived URL: %s", body)
	}
	if !strings.Contains(body, `/connections/My%20Server/`) {
		t.Fatalf("body missing the properly-escaped connection name: %s", body)
	}
}

func TestBlobsHandler_List_EmptyRoot(t *testing.T) {
	store := seedConnection(t, newFakeXoluBlobs(t).URL)
	h := &blobsHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/blobs", nil)
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "This folder is empty") {
		t.Fatalf("body missing the empty-folder message: %s", rec.Body.String())
	}
}

func TestBlobsHandler_UploadThenListThenDownload(t *testing.T) {
	store := seedConnection(t, newFakeXoluBlobs(t).URL)
	h := &blobsHandler{store: store}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("path", "photos")
	fw, _ := mw.CreateFormFile("file", "vacation.jpg")
	_, _ = fw.Write([]byte("fake jpeg bytes"))
	_ = mw.Close()

	uploadReq := httptest.NewRequest(http.MethodPost, "/connections/test/blob-upload", &buf)
	uploadReq.Header.Set("Content-Type", mw.FormDataContentType())
	uploadReq.SetPathValue("name", "test")
	uploadRec := httptest.NewRecorder()
	h.Upload(uploadRec, uploadReq)

	if uploadRec.Code != http.StatusSeeOther {
		t.Fatalf("upload status = %d, want %d; body = %s", uploadRec.Code, http.StatusSeeOther, uploadRec.Body.String())
	}
	if loc := uploadRec.Header().Get("Location"); loc != "/connections/test/blobs/photos" {
		t.Fatalf("upload Location = %q, want redirect back to the folder it was uploaded into", loc)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/connections/test/blobs/photos", nil)
	listReq.SetPathValue("name", "test")
	listReq.SetPathValue("path", "photos")
	listRec := httptest.NewRecorder()
	h.List(listRec, listReq)
	if !strings.Contains(listRec.Body.String(), "vacation.jpg") {
		t.Fatalf("uploaded file not shown in the listing: %s", listRec.Body.String())
	}

	dlReq := httptest.NewRequest(http.MethodGet, "/connections/test/blob-download?key="+url.QueryEscape("photos:vacation.jpg"), nil)
	dlReq.SetPathValue("name", "test")
	dlRec := httptest.NewRecorder()
	h.Download(dlRec, dlReq)
	if dlRec.Code != http.StatusOK {
		t.Fatalf("download status = %d, want %d", dlRec.Code, http.StatusOK)
	}
	if dlRec.Body.String() != "fake jpeg bytes" {
		t.Fatalf("downloaded content = %q, want the uploaded bytes back exactly", dlRec.Body.String())
	}
	if !strings.Contains(dlRec.Header().Get("Content-Disposition"), "vacation.jpg") {
		t.Fatalf("Content-Disposition = %q, want it to name the file", dlRec.Header().Get("Content-Disposition"))
	}
}

func TestBlobsHandler_DeleteFile(t *testing.T) {
	server := newFakeXoluBlobs(t)
	store := seedConnection(t, server.URL)
	h := &blobsHandler{store: store}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("path", "")
	fw, _ := mw.CreateFormFile("file", "notes.txt")
	_, _ = fw.Write([]byte("hi"))
	_ = mw.Close()
	uploadReq := httptest.NewRequest(http.MethodPost, "/connections/test/blob-upload", &buf)
	uploadReq.Header.Set("Content-Type", mw.FormDataContentType())
	uploadReq.SetPathValue("name", "test")
	h.Upload(httptest.NewRecorder(), uploadReq)

	form := url.Values{"key": {"notes.txt"}}
	req := httptest.NewRequest(http.MethodPost, "/connections/test/blob-delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()
	h.DeleteFile(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/connections/test/blobs", nil)
	listReq.SetPathValue("name", "test")
	listRec := httptest.NewRecorder()
	h.List(listRec, listReq)
	if strings.Contains(listRec.Body.String(), "notes.txt") {
		t.Fatalf("deleted file still shown in the listing: %s", listRec.Body.String())
	}
}

func TestBlobsHandler_NewFolder_ThenAppearsEmpty(t *testing.T) {
	store := seedConnection(t, newFakeXoluBlobs(t).URL)
	h := &blobsHandler{store: store}

	form := url.Values{"path": {""}, "name": {"empty-one"}}
	req := httptest.NewRequest(http.MethodPost, "/connections/test/blob-new-folder", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()
	h.NewFolder(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/connections/test/blobs", nil)
	listReq.SetPathValue("name", "test")
	listRec := httptest.NewRecorder()
	h.List(listRec, listReq)
	if !strings.Contains(listRec.Body.String(), "empty-one") {
		t.Fatalf("new folder not shown in the listing: %s", listRec.Body.String())
	}
}

func TestBlobsHandler_DeleteFolder_EmptySucceeds(t *testing.T) {
	store := seedConnection(t, newFakeXoluBlobs(t).URL)
	h := &blobsHandler{store: store}

	create := url.Values{"path": {""}, "name": {"gone-soon"}}
	createReq := httptest.NewRequest(http.MethodPost, "/connections/test/blob-new-folder", strings.NewReader(create.Encode()))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createReq.SetPathValue("name", "test")
	h.NewFolder(httptest.NewRecorder(), createReq)

	del := url.Values{"path": {""}, "name": {"gone-soon"}}
	delReq := httptest.NewRequest(http.MethodPost, "/connections/test/blob-delete-folder", strings.NewReader(del.Encode()))
	delReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	delReq.SetPathValue("name", "test")
	delRec := httptest.NewRecorder()
	h.DeleteFolder(delRec, delReq)

	if delRec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body = %s", delRec.Code, http.StatusSeeOther, delRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/connections/test/blobs", nil)
	listReq.SetPathValue("name", "test")
	listRec := httptest.NewRecorder()
	h.List(listRec, listReq)
	if strings.Contains(listRec.Body.String(), "gone-soon") {
		t.Fatalf("deleted folder still shown: %s", listRec.Body.String())
	}
}

func TestBlobsHandler_DeleteFolder_NonEmptyRefused(t *testing.T) {
	store := seedConnection(t, newFakeXoluBlobs(t).URL)
	h := &blobsHandler{store: store}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("path", "docs")
	fw, _ := mw.CreateFormFile("file", "readme.txt")
	_, _ = fw.Write([]byte("hi"))
	_ = mw.Close()
	uploadReq := httptest.NewRequest(http.MethodPost, "/connections/test/blob-upload", &buf)
	uploadReq.Header.Set("Content-Type", mw.FormDataContentType())
	uploadReq.SetPathValue("name", "test")
	h.Upload(httptest.NewRecorder(), uploadReq)

	del := url.Values{"path": {""}, "name": {"docs"}}
	delReq := httptest.NewRequest(http.MethodPost, "/connections/test/blob-delete-folder", strings.NewReader(del.Encode()))
	delReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	delReq.SetPathValue("name", "test")
	delRec := httptest.NewRecorder()
	h.DeleteFolder(delRec, delReq)

	if delRec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d — a non-empty folder must be refused, not silently deleted", delRec.Code, http.StatusConflict)
	}
}

func TestBlobsHandler_Download_UnknownKey(t *testing.T) {
	store := seedConnection(t, newFakeXoluBlobs(t).URL)
	h := &blobsHandler{store: store}

	req := httptest.NewRequest(http.MethodGet, "/connections/test/blob-download?key=nope.txt", nil)
	req.SetPathValue("name", "test")
	rec := httptest.NewRecorder()
	h.Download(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestBlobsHandler_List_UnknownConnection(t *testing.T) {
	h := &blobsHandler{store: newTestStore(t)}
	req := httptest.NewRequest(http.MethodGet, "/connections/nope/blobs", nil)
	req.SetPathValue("name", "nope")
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
