// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package seedapply

import (
	"context"
	"encoding/json"
	"fmt"

	xclient "github.com/ha1tch/xolu/pkg/client"
)

// CreatedKind names one kind of thing an apply run created.
type CreatedKind string

const (
	CreatedEntity CreatedKind = "entity"
	CreatedFSMDef CreatedKind = "fsm_def"
	// CreatedSchema, CreatedDXPDef, CreatedBalAccount,
	// CreatedBalTransfer, CreatedCalCalendar, CreatedCalBooking, and
	// CreatedRaw are all tracked for reporting but are NOT deletable
	// — confirmed directly against xolu's own public client library:
	// no delete/cancel/remove method exists for a schema, a DXP def,
	// a bal account or transfer, a cal calendar or booking, or
	// anything reached through a raw step. Rollback for these is
	// permanently impossible through this client, not merely
	// unimplemented — see Rollback's own doc comment for why this
	// matters to the safety design.
	CreatedSchema      CreatedKind = "schema"
	CreatedDXPDef      CreatedKind = "dxp_def"
	CreatedBalAccount  CreatedKind = "bal_account"
	CreatedBalTransfer CreatedKind = "bal_transfer"
	CreatedCalCalendar CreatedKind = "cal_calendar"
	CreatedCalBooking  CreatedKind = "cal_booking"
	CreatedRaw         CreatedKind = "raw"
)

// deletableKinds is the closed, confirmed set of kinds Rollback can
// actually undo — entities and FSM definitions only. Every other kind
// is tracked purely for an honest report, never attempted.
var deletableKinds = map[CreatedKind]bool{
	CreatedEntity: true,
	CreatedFSMDef: true,
}

// Created is one thing an apply run made, in creation order — the
// record Rollback walks in reverse.
type Created struct {
	Kind CreatedKind `json:"kind"`
	// EntityType is set for CreatedEntity and CreatedSchema.
	EntityType string `json:"entityType,omitempty"`
	// ID is the numeric id xolu assigned, for kinds that get one
	// (entities, FSM defs). Zero when not applicable.
	ID int64 `json:"id,omitempty"`
	// StringID is the client-chosen string identifier for kinds that
	// use one instead of a server-assigned numeric id — bal's own
	// account_id/transfer_id, cal's own calendar_id/booking_id.
	// Empty when not applicable.
	StringID string `json:"stringId,omitempty"`
	// Key is the seed-local symbolic key this creation corresponds
	// to, for CreatedEntity records that declared a "_key" — used to
	// resolve $ref fields in the second pass, not needed after that.
	Key string `json:"key,omitempty"`
}

// Deletable reports whether Rollback can actually undo this specific
// creation.
func (c Created) Deletable() bool {
	return deletableKinds[c.Kind]
}

// RollbackReport is what actually happened when Rollback ran — what
// it removed, and, just as importantly, what it could not, because
// xolu's own client offers no way to.
type RollbackReport struct {
	Removed        []Created         `json:"removed"`
	RemoveFailures []RollbackFailure `json:"removeFailures"`
	// NotDeletable is everything Rollback never attempted because no
	// delete capability exists for that kind at all — distinct from a
	// RemoveFailure (attempted, failed) and worth showing separately
	// so the person understands these aren't retryable.
	NotDeletable []Created `json:"notDeletable"`
}

// RollbackFailure is one Created record Rollback attempted to remove
// and could not — a real API error, not a missing capability.
type RollbackFailure struct {
	Created Created
	Err     error
}

// MarshalJSON is implemented explicitly because Go's error interface
// has no exported fields — json.Marshal's own default struct encoding
// would silently produce {} for Err, losing the actual failure
// message entirely on the wire. Caught directly: a live run against a
// real server showed the rollback UI reporting "Removed 0 item(s)"
// with no visible reason, traced to this same class of gap on the
// sibling Created/RollbackReport fields (missing json tags, not a
// missing MarshalJSON) before this one was checked and fixed too.
func (f RollbackFailure) MarshalJSON() ([]byte, error) {
	var errMsg string
	if f.Err != nil {
		errMsg = f.Err.Error()
	}
	return json.Marshal(struct {
		Created Created `json:"created"`
		Error   string  `json:"error"`
	}{Created: f.Created, Error: errMsg})
}

// Rollback attempts to undo everything in created, in reverse
// creation order (last created, first removed — respects dependency
// direction without needing to reason about it explicitly). Only
// entities and FSM definitions can ever be removed this way; every
// other kind (schemas, DXP defs, bal, cal, raw) is permanently
// non-deletable through xolu's own public client and is reported in
// NotDeletable rather than attempted.
//
// This is exactly why the seed system's own empty-connection
// precondition (checked before any apply begins, see emptiness.go)
// matters as much as it does: for the non-deletable kinds, refusing
// to start against a non-empty tenant is the only real safety net —
// rollback cannot clean up what it structurally cannot remove.
func Rollback(ctx context.Context, c *xclient.Client, created []Created) *RollbackReport {
	report := &RollbackReport{}
	for i := len(created) - 1; i >= 0; i-- {
		item := created[i]
		if !item.Deletable() {
			report.NotDeletable = append(report.NotDeletable, item)
			continue
		}
		var err error
		switch item.Kind {
		case CreatedEntity:
			err = c.Delete(ctx, item.EntityType, item.ID)
		case CreatedFSMDef:
			err = c.DeleteMachineDef(ctx, item.ID)
		default:
			// Unreachable given deletableKinds above, but a
			// programming error here (a new kind added to
			// deletableKinds without a case here) must surface
			// loudly, not silently skip.
			err = fmt.Errorf("seedapply: rollback: no handler for deletable kind %q", item.Kind)
		}
		if err != nil {
			report.RemoveFailures = append(report.RemoveFailures, RollbackFailure{Created: item, Err: err})
			continue
		}
		report.Removed = append(report.Removed, item)
	}
	return report
}
