// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package connstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// fileData is the on-disk shape of connections.json. Wrapped in a struct
// (rather than a bare map at the top level) so a schema version or other
// top-level field can be added later without breaking the format.
type fileData struct {
	Connections map[string]Connection `json:"connections"`
}

// FileBackend is the Store implementation that persists full Connection
// values, including the token, to a single plaintext JSON file. Always
// available — no external dependency — and the default backend.
type FileBackend struct {
	path string
	mu   sync.Mutex // guards read-modify-write; see fileBackendPath doc
}

// NewFileBackend returns a FileBackend backed by <dir>/connections.json.
// The directory must already exist (config.Dir creates it).
func NewFileBackend(dir string) *FileBackend {
	return &FileBackend{path: filepath.Join(dir, "connections.json")}
}

// fileBackendPath's mutex guards concurrent access within one xoluman
// process only. xoluman is a single-user local tool with no expectation
// of multiple processes sharing one config directory concurrently; a
// cross-process lock is not implemented.

func (f *FileBackend) load() (fileData, error) {
	data, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return fileData{Connections: map[string]Connection{}}, nil
	}
	if err != nil {
		return fileData{}, err
	}
	var fd fileData
	if err := json.Unmarshal(data, &fd); err != nil {
		return fileData{}, err
	}
	if fd.Connections == nil {
		fd.Connections = map[string]Connection{}
	}
	return fd, nil
}

func (f *FileBackend) save(fd fileData) error {
	data, err := json.MarshalIndent(fd, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}

func (f *FileBackend) List(ctx context.Context) ([]Connection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	fd, err := f.load()
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(fd.Connections))
	for _, c := range fd.Connections {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (f *FileBackend) Get(ctx context.Context, name string) (Connection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	fd, err := f.load()
	if err != nil {
		return Connection{}, err
	}
	c, ok := fd.Connections[name]
	if !ok {
		return Connection{}, ErrNotFound
	}
	return c, nil
}

func (f *FileBackend) Save(ctx context.Context, conn Connection) (bool, error) {
	if err := validate(conn); err != nil {
		return false, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	fd, err := f.load()
	if err != nil {
		return false, err
	}
	_, existed := fd.Connections[conn.Name]
	fd.Connections[conn.Name] = conn
	if err := f.save(fd); err != nil {
		return false, err
	}
	return !existed, nil
}

func (f *FileBackend) Delete(ctx context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	fd, err := f.load()
	if err != nil {
		return err
	}
	if _, ok := fd.Connections[name]; !ok {
		return ErrNotFound
	}
	delete(fd.Connections, name)
	return f.save(fd)
}
