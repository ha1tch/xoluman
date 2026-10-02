// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package seedapply executes a loaded seed (internal/seeds) against a
// real, connected xolu tenant. Kept separate from internal/seeds
// deliberately: seeds is a pure format definition with no xolu
// dependency at all (unit-testable without any client or server);
// seedapply is the part that actually talks to xolu.
//
// The central design problem this package exists to solve: a data
// step's entities reference each other (a deal's owner, a contact's
// company), but none of them have real IDs until xolu assigns them —
// REF fields can't be written at authoring time the way plain field
// values can. The solution is a seed-local symbolic key per entity
// (the record's own "_key") and a two-pass apply: pass one creates
// every entity with REF fields omitted, recording seed-local key ->
// real (entity type, id) as they're assigned; pass two patches every
// omitted REF field back in, now that every key resolves to something
// real. Order never matters within or across data steps — a "$ref"
// can point at a key defined earlier or later in the same seed.
package seedapply

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
)

// RawEntity is one JSONL line from a data step, parsed but not yet
// split into immediate/deferred fields.
type RawEntity struct {
	// Key is this entity's own seed-local symbolic key (from "_key"),
	// used by other entities' $ref fields to point at it. Empty when
	// the record declares no _key — valid; it just means nothing else
	// in the seed can reference this particular entity.
	Key string
	// Fields is every field except _key, values unmodified — $ref
	// markers are still present, split out by SplitFields.
	Fields map[string]any
}

// ParseJSONLEntities parses a data step's own file: one JSON object
// per non-blank line, "_key" pulled out of Fields into Key.
func ParseJSONLEntities(data []byte) ([]RawEntity, error) {
	var entities []RawEntity
	scanner := bufio.NewScanner(bytes.NewReader(data))
	// Entity records can be arbitrarily large (many fields, nested
	// structures) -- the default 64KB scanner buffer is a real limit
	// worth raising explicitly rather than silently truncating a
	// legitimate long line.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 4*1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			return nil, fmt.Errorf("seedapply: line %d: invalid JSON: %w", lineNum, err)
		}
		var key string
		if k, ok := raw["_key"]; ok {
			s, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("seedapply: line %d: \"_key\" must be a string", lineNum)
			}
			key = s
			delete(raw, "_key")
		}
		entities = append(entities, RawEntity{Key: key, Fields: raw})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("seedapply: scanning JSONL: %w", err)
	}
	return entities, nil
}

// SplitFields separates one entity's own fields into immediate (safe
// to send as-is on the first pass) and deferred (a top-level $ref
// marker, resolved on the second pass once every seed-local key has a
// real id). Only top-level field values are checked for the exact
// {"$ref": "key"} shape — a $ref nested inside an array or a deeper
// object is not supported by this first version; such a field is left
// in immediate as-is and will fail xolu's own REF validation
// server-side if it wasn't meant to be literal data, which is an
// honest, visible failure rather than a silently wrong resolution.
func SplitFields(fields map[string]any) (immediate map[string]any, deferred map[string]string) {
	immediate = make(map[string]any, len(fields))
	deferred = make(map[string]string)
	for k, v := range fields {
		if key, ok := refKey(v); ok {
			deferred[k] = key
			continue
		}
		immediate[k] = v
	}
	return immediate, deferred
}

// refKey reports whether v is exactly {"$ref": "<key>"} — a JSON
// object with precisely one key, "$ref", whose value is a string.
func refKey(v any) (string, bool) {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 1 {
		return "", false
	}
	key, ok := m["$ref"].(string)
	return key, ok
}
