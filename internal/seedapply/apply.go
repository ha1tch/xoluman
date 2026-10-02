// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package seedapply

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/seeds"
)

// refTarget is what a seed-local key resolves to once its entity has
// actually been created.
type refTarget struct {
	entityType string
	id         int64
}

// asRefValue renders a resolved target in xolu's own real REF shape —
// confirmed directly against a live server response earlier in this
// project's own work, not assumed: {"entity": <type>, "id": <id>,
// "type": "REF"}.
func (t refTarget) asRefValue() map[string]any {
	return map[string]any{"entity": t.entityType, "id": t.id, "type": "REF"}
}

// pendingPatch is one entity whose creation (phase A) omitted one or
// more REF fields, to be patched in during phase B once every
// seed-local key in the whole seed has resolved.
type pendingPatch struct {
	entityType string
	id         int64
	deferred   map[string]string // field name -> seed-local key
}

// Result is the outcome of one Apply run — every Created record (used
// for both the person-facing summary and, on failure, as Rollback's
// own input), and, if the run stopped early, the error and which step
// index it stopped at.
type Result struct {
	Created      []Created
	Err          error
	FailedAtStep int // -1 when Err is nil
}

// Apply runs every step of a loaded seed against c, in manifest
// order, then resolves every deferred REF field in a second pass. It
// stops at the first failing step — Result.Err and
// Result.FailedAtStep report exactly where — rather than continuing
// past a failure into steps that may depend on what just failed to
// create. Result.Created is populated up through whatever succeeded,
// suitable as-is for Rollback regardless of how far the run got.
//
// Callers are responsible for the tenant-emptiness precondition (see
// emptiness.go) before ever calling Apply — Apply itself does not
// check it, since that check's own configurability (on by default,
// switchable) is a decision made once per apply attempt, not
// something to bake into the executor itself.
func Apply(ctx context.Context, c *xclient.Client, ls *seeds.LoadedSeed) *Result {
	result := &Result{FailedAtStep: -1}
	keys := map[string]refTarget{}
	var pending []pendingPatch

	for i, step := range ls.Manifest.Steps {
		data, err := ls.ReadStepFile(step)
		if err != nil {
			result.Err = err
			result.FailedAtStep = i
			return result
		}

		var created []Created
		var stepPending []pendingPatch
		switch step.Type {
		case seeds.StepSchema:
			created, err = applySchema(ctx, c, step, data)
		case seeds.StepData:
			created, stepPending, err = applyData(ctx, c, step, data, keys)
		case seeds.StepFSM:
			created, err = applyFSM(ctx, c, data)
		case seeds.StepDXPDef:
			created, err = applyDXPDef(ctx, c, data)
		case seeds.StepBalDefine:
			created, err = applyBalDefine(ctx, c, data)
		case seeds.StepBalTransfer:
			created, err = applyBalTransfer(ctx, c, data)
		case seeds.StepCalCreateCalendar:
			created, err = applyCalCreateCalendar(ctx, c, data)
		case seeds.StepCalPropose:
			created, err = applyCalPropose(ctx, c, data)
		case seeds.StepRaw:
			created, err = applyRaw(ctx, c, step, data)
		default:
			err = fmt.Errorf("seedapply: step %d: no executor for step type %q", i, step.Type)
		}

		result.Created = append(result.Created, created...)
		for _, cr := range created {
			if cr.Kind == CreatedEntity && cr.Key != "" {
				keys[cr.Key] = refTarget{entityType: cr.EntityType, id: cr.ID}
			}
		}
		pending = append(pending, stepPending...)

		if err != nil {
			result.Err = fmt.Errorf("seedapply: step %d (%s): %w", i, step.Type, err)
			result.FailedAtStep = i
			return result
		}
	}

	// Phase B: every seed-local key referenced anywhere must resolve
	// by now — every entity in the whole seed has been created. A key
	// that still doesn't resolve here is a genuine seed authoring
	// error (a $ref to a key that was never declared), not something
	// to patch around silently.
	for _, p := range pending {
		patch := make(map[string]any, len(p.deferred))
		for field, key := range p.deferred {
			target, ok := keys[key]
			if !ok {
				result.Err = fmt.Errorf("seedapply: entity %s/%d field %q references undefined key %q", p.entityType, p.id, field, key)
				result.FailedAtStep = len(ls.Manifest.Steps) // past the last real step -- this is the resolution phase
				return result
			}
			patch[field] = target.asRefValue()
		}
		if _, err := c.Patch(ctx, p.entityType, p.id, patch); err != nil {
			result.Err = fmt.Errorf("seedapply: resolving refs on %s/%d: %w", p.entityType, p.id, err)
			result.FailedAtStep = len(ls.Manifest.Steps)
			return result
		}
	}

	return result
}

func applySchema(ctx context.Context, c *xclient.Client, step seeds.Step, data []byte) ([]Created, error) {
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("invalid schema JSON: %w", err)
	}
	if err := c.DefineEntitySchema(ctx, step.EntityType, schema); err != nil {
		return nil, err
	}
	return []Created{{Kind: CreatedSchema, EntityType: step.EntityType}}, nil
}

func applyData(ctx context.Context, c *xclient.Client, step seeds.Step, data []byte, _ map[string]refTarget) ([]Created, []pendingPatch, error) {
	entities, err := ParseJSONLEntities(data)
	if err != nil {
		return nil, nil, err
	}
	var created []Created
	var pending []pendingPatch
	for lineIdx, e := range entities {
		immediate, deferred := SplitFields(e.Fields)
		result, err := c.Create(ctx, step.EntityType, immediate)
		if err != nil {
			return created, pending, fmt.Errorf("line %d (key %q): %w", lineIdx+1, e.Key, err)
		}
		created = append(created, Created{Kind: CreatedEntity, EntityType: step.EntityType, ID: result.ID, Key: e.Key})
		if len(deferred) > 0 {
			pending = append(pending, pendingPatch{entityType: step.EntityType, id: result.ID, deferred: deferred})
		}
	}
	return created, pending, nil
}

func applyFSM(ctx context.Context, c *xclient.Client, data []byte) ([]Created, error) {
	var spec xclient.MachineSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("invalid MachineSpec JSON: %w", err)
	}
	result, err := c.CreateMachineDef(ctx, spec)
	if err != nil {
		return nil, err
	}
	return []Created{{Kind: CreatedFSMDef, ID: result.ID}}, nil
}

func applyDXPDef(ctx context.Context, c *xclient.Client, data []byte) ([]Created, error) {
	var req xclient.DxpDefCreateRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid DxpDefCreateRequest JSON: %w", err)
	}
	result, err := c.DxpDefCreate(ctx, req)
	if err != nil {
		return nil, err
	}
	return []Created{{Kind: CreatedDXPDef, ID: result.ID}}, nil
}

func applyBalDefine(ctx context.Context, c *xclient.Client, data []byte) ([]Created, error) {
	var req xclient.BalDefineRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid BalDefineRequest JSON: %w", err)
	}
	result, err := c.BalDefine(ctx, req)
	if err != nil {
		return nil, err
	}
	return []Created{{Kind: CreatedBalAccount, StringID: result.AccountID}}, nil
}

func applyBalTransfer(ctx context.Context, c *xclient.Client, data []byte) ([]Created, error) {
	var req xclient.BalTransferRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid BalTransferRequest JSON: %w", err)
	}
	result, err := c.BalTransfer(ctx, req)
	if err != nil {
		return nil, err
	}
	return []Created{{Kind: CreatedBalTransfer, StringID: result.TransferID}}, nil
}

func applyCalCreateCalendar(ctx context.Context, c *xclient.Client, data []byte) ([]Created, error) {
	var req xclient.CalCreateCalendarRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid CalCreateCalendarRequest JSON: %w", err)
	}
	result, err := c.CalCreateCalendar(ctx, req)
	if err != nil {
		return nil, err
	}
	return []Created{{Kind: CreatedCalCalendar, StringID: result.CalendarID}}, nil
}

func applyCalPropose(ctx context.Context, c *xclient.Client, data []byte) ([]Created, error) {
	var req xclient.CalProposeRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid CalProposeRequest JSON: %w", err)
	}
	result, err := c.CalPropose(ctx, req)
	if err != nil {
		return nil, err
	}
	return []Created{{Kind: CreatedCalBooking, StringID: result.BookingID}}, nil
}

func applyRaw(ctx context.Context, c *xclient.Client, step seeds.Step, data []byte) ([]Created, error) {
	_, err := c.Raw(ctx, step.Method, step.Path, "application/json", strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	return []Created{{Kind: CreatedRaw}}, nil
}
