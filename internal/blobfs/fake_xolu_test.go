// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package blobfs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ha1tch/xolu/pkg/client"
)

// fakeXolu is a minimal but genuinely stateful in-memory xolu stand-in
// — real blob-key prefix matching and real xoluman_blob_folder entity
// CRUD, since blobfs's reconciliation logic depends on their actual
// interaction (materialize, then see the materialized entity on the
// next call; delete, then see it gone), which a call-by-call mock
// can't represent honestly.
type fakeXolu struct {
	mu       sync.Mutex
	blobKeys []string // just keys — size/content-type/sha256 aren't exercised by any blobfs logic
	folders  map[int64]map[string]any
	nextID   int64
	server   *httptest.Server
}

func newFakeXolu(t *testing.T) *fakeXolu {
	t.Helper()
	f := &fakeXolu{folders: make(map[int64]map[string]any)}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/blob", func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Query().Get("prefix")
		f.mu.Lock()
		defer f.mu.Unlock()
		var blobs []map[string]any
		for _, k := range f.blobKeys {
			if strings.HasPrefix(k, prefix) {
				blobs = append(blobs, map[string]any{"key": k, "sha256": "x", "size": float64(1), "stored_at": "2026-08-04T00:00:00Z"})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"tenant": "default", "prefix": prefix, "count": len(blobs), "blobs": blobs})
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
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       data,
			"pagination": map[string]any{"page": 1, "per_page": len(data) + 1, "total_items": len(data), "total_pages": 1},
		})
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

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeXolu) client() *client.Client {
	return client.New(f.server.URL)
}

// seedBlobs directly injects blob keys, bypassing BlobPut entirely —
// blobfs's own logic never calls BlobPut, only BlobList, so there's
// nothing to gain from round-tripping through the real Put path here.
func (f *fakeXolu) seedBlobs(keys ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blobKeys = append(f.blobKeys, keys...)
}

// folderCount and explicitFolderNamed are small test-only introspection
// helpers, not part of blobfs's real API.
func (f *fakeXolu) folderCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.folders)
}

func (f *fakeXolu) folderNamed(name string) (map[string]any, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, doc := range f.folders {
		if doc["name"] == name {
			return doc, true
		}
	}
	return nil, false
}
