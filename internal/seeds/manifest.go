// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package seeds defines xoluman's seed package format — a directory
// carrying a manifest (this file), schemas, data, and other xolu
// primitive definitions that can be previewed and applied to a
// connected tenant in one action.
//
// Design (agreed, see project discussion): a directory, not a single
// file — seed.json manifest, schemas/*.json (real JSON Schema),
// data/*.jsonl (one entity per line, symbolic $ref keys resolved in a
// second pass rather than real IDs, since IDs don't exist at authoring
// time), fsms/*.json (a real MachineSpec), and one file per step for
// every other primitive step. manual and preview_images point at
// files within the same directory, rendered during preview before
// anything is ever applied.
//
// Step kinds are typed wherever xolu's own client library already has
// a real, typed request shape — confirmed directly against xolu's
// source, not assumed: entities, FSM, DXP defs, bal, and cal all have
// one. loc, obj, and ts do not (confirmed no client-library coverage
// for loc/obj at all, and ts is read-only in the client) but all
// three have real, working REST endpoints (obj's own /attach and
// /promote observed succeeding directly; ts's own /ts/events/batch
// confirmed against the real server handler) — so those three use the
// raw step kind, a plain {method, path, file} passthrough, until xolu
// ships typed client support. Upgrading a step kind from raw to typed
// later changes nothing about the manifest format itself; only the
// executor's own internal handling of that one step kind changes.
package seeds

import (
	"encoding/json"
	"fmt"
)

// FormatVersion is the only manifest format version this package
// currently understands. A manifest declaring anything else is
// rejected outright rather than guessed at — an unknown future format
// is not safe to partially interpret.
const FormatVersion = 1

// StepType names one kind of action a seed step performs.
type StepType string

const (
	// StepSchema registers a JSON Schema for one entity type. File is
	// a real JSON Schema document; EntityType names the entity type
	// it applies to.
	StepSchema StepType = "schema"
	// StepData creates entity rows of one type. File is JSONL — one
	// entity per line, REF fields written as {"$ref": "<seed-local
	// key>"} rather than a real xolu REF, resolved in a second pass
	// once every step has run and every seed-local key has a real ID.
	StepData StepType = "data"
	// StepFSM registers one FSM definition. File decodes directly
	// into xclient.MachineSpec.
	StepFSM StepType = "fsm"
	// StepDXPDef registers one DXP definition. File decodes directly
	// into xclient.DxpDefCreateRequest.
	StepDXPDef StepType = "dxp_def"
	// StepBalDefine defines one bal account. File decodes directly
	// into xclient.BalDefineRequest.
	StepBalDefine StepType = "bal_define"
	// StepBalTransfer performs one bal transfer. File decodes
	// directly into xclient.BalTransferRequest.
	StepBalTransfer StepType = "bal_transfer"
	// StepCalCreateCalendar creates one cal calendar. File decodes
	// directly into xclient.CalCreateCalendarRequest.
	StepCalCreateCalendar StepType = "cal_create_calendar"
	// StepCalPropose proposes one cal booking. File decodes directly
	// into xclient.CalProposeRequest.
	StepCalPropose StepType = "cal_propose"
	// StepRaw is a plain REST passthrough — Method, Path, and File
	// (the raw request body) posted as-is, no structural validation
	// beyond "is this valid JSON" before it leaves xoluman. The only
	// step kind usable for loc, obj, and ts today; see this package's
	// own doc comment for why.
	StepRaw StepType = "raw"
)

// knownStepTypes is the complete, closed set — a manifest declaring
// anything outside this set is rejected at parse time, not discovered
// later as an executor lookup failure.
var knownStepTypes = map[StepType]bool{
	StepSchema:            true,
	StepData:              true,
	StepFSM:               true,
	StepDXPDef:            true,
	StepBalDefine:         true,
	StepBalTransfer:       true,
	StepCalCreateCalendar: true,
	StepCalPropose:        true,
	StepRaw:               true,
}

// requiresEntityType is the subset of step types that must carry a
// non-empty EntityType.
var requiresEntityType = map[StepType]bool{
	StepSchema: true,
	StepData:   true,
}

// Step is one action a seed performs, in manifest order. Order matters
// for readability and for schema-before-data within one entity type,
// but never for cross-entity REF resolution — see StepData's own doc
// comment for why that's a separate, order-independent pass.
type Step struct {
	Type StepType `json:"type"`
	// EntityType is required for StepSchema and StepData, and
	// meaningless for every other step type.
	EntityType string `json:"entity_type,omitempty"`
	// File is the path, relative to the seed directory's own root, to
	// this step's payload. Required for every step type.
	File string `json:"file"`
	// Method and Path are used only by StepRaw — the HTTP method and
	// the full, absolute request path, exactly as xolu's own client
	// Raw method sends it: no "/api/v1" or "/api/v2" prefix, no
	// tenant segment, is added automatically. A raw step's Path must
	// be the complete real path (e.g.
	// "/api/v1/tenant/acme/ts/events/batch"), not a short suffix —
	// confirmed directly against xclient.Client.Raw's own
	// implementation, which does a bare baseURL+path concatenation.
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
}

// Manifest is one seed package's own seed.json, fully parsed and
// structurally validated — every step has a known type and its own
// type's required fields. Manifest does not itself check that the
// files Step.File names actually exist; that is Load's own concern
// (manifest.go stays a pure, filesystem-free parser, deliberately,
// so it can be unit tested without a real directory on disk).
type Manifest struct {
	FormatVersion int      `json:"format_version"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	// Manual is a path, relative to the seed directory's own root, to
	// a markdown file rendered in the preview UI. Optional.
	Manual string `json:"manual,omitempty"`
	// PreviewImages are paths, relative to the seed directory's own
	// root, shown as a gallery in the preview UI before anything is
	// applied. Optional.
	PreviewImages []string `json:"preview_images,omitempty"`
	Steps         []Step   `json:"steps"`
}

// ParseManifest parses and structurally validates raw seed.json bytes.
// It never touches a filesystem — see Manifest's own doc comment for
// why that separation is deliberate.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("seeds: invalid manifest JSON: %w", err)
	}
	if err := m.validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Manifest) validate() error {
	if m.FormatVersion != FormatVersion {
		return fmt.Errorf("seeds: unsupported format_version %d (this xoluman understands version %d)", m.FormatVersion, FormatVersion)
	}
	if m.ID == "" {
		return fmt.Errorf("seeds: manifest missing required field \"id\"")
	}
	if m.Name == "" {
		return fmt.Errorf("seeds: manifest missing required field \"name\"")
	}
	if len(m.Steps) == 0 {
		return fmt.Errorf("seeds: manifest %q has no steps", m.ID)
	}
	for i, s := range m.Steps {
		if err := s.validate(i); err != nil {
			return fmt.Errorf("seeds: manifest %q: %w", m.ID, err)
		}
	}
	return nil
}

func (s Step) validate(index int) error {
	if s.Type == "" {
		return fmt.Errorf("step %d: missing required field \"type\"", index)
	}
	if !knownStepTypes[s.Type] {
		return fmt.Errorf("step %d: unknown step type %q", index, s.Type)
	}
	if s.File == "" {
		return fmt.Errorf("step %d (%s): missing required field \"file\"", index, s.Type)
	}
	if requiresEntityType[s.Type] && s.EntityType == "" {
		return fmt.Errorf("step %d (%s): missing required field \"entity_type\"", index, s.Type)
	}
	if s.Type == StepRaw {
		if s.Method == "" {
			return fmt.Errorf("step %d (raw): missing required field \"method\"", index)
		}
		if s.Path == "" {
			return fmt.Errorf("step %d (raw): missing required field \"path\"", index)
		}
	}
	return nil
}
