// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package seedapply

import (
	"context"
	"fmt"
	"sort"

	xclient "github.com/ha1tch/xolu/pkg/client"
)

// FindingStatus is what CheckEmpty found for one primitive group.
type FindingStatus string

const (
	// FindingEmpty means this group was checked and confirmed to have
	// no data.
	FindingEmpty FindingStatus = "empty"
	// FindingHasData means this group has at least some data — this
	// alone is enough to fail the whole check.
	FindingHasData FindingStatus = "has_data"
	// FindingCheckFailed means the check itself could not complete —
	// a transport error, a non-2xx response, or similar. Treated the
	// same as FindingHasData for the overall Empty verdict: "we don't
	// know" is not "it's empty."
	FindingCheckFailed FindingStatus = "check_failed"
)

// Finding is what CheckEmpty found for one primitive group.
type Finding struct {
	Primitive string
	Status    FindingStatus
	// Detail is a short, human-readable explanation — what was found,
	// or what error occurred.
	Detail string
}

// EmptinessResult is the full report from CheckEmpty.
//
// Empty is true only when the tenant genuinely has no data in any
// store xolu itself tracks. As of xolu v0.30.32 (XOT213/XOT216) this
// is answered by a single, server-side, authoritative call
// (Client.TenantSummary) rather than xoluman polling each primitive
// separately — the earlier version of this file made eight separate
// round trips and, for one of them (obj), could never get a real
// answer at all, since no enumeration endpoint existed anywhere in
// xolu's own server for it. Both of those limitations are gone now:
// TenantSummary covers every store the server itself knows about,
// obj included, in one request — and it also covers blob, which the
// earlier per-primitive implementation never checked at all, a real
// gap in xoluman's own prior coverage, not just an xolu one.
type EmptinessResult struct {
	Empty    bool
	Findings []Finding
}

// CheckEmpty confirms whether a connected tenant has any data at all,
// via a single call to xolu's own tenant-summary endpoint. See
// EmptinessResult's own doc comment for what changed and why.
func CheckEmpty(ctx context.Context, c *xclient.Client) *EmptinessResult {
	summary, err := c.TenantSummary(ctx)
	if err != nil {
		return &EmptinessResult{
			Empty: false,
			Findings: []Finding{
				{Primitive: "tenant-summary", Status: FindingCheckFailed, Detail: err.Error()},
			},
		}
	}
	return &EmptinessResult{Empty: summary.Empty, Findings: summaryFindings(summary)}
}

// summaryFindings turns one TenantSummary into a Finding per
// top-level group (primary, loc, obj, ts, cal_index, bal_rollup,
// blob) — the same grouping xolu's own server uses, not a
// finer-grained, independently-maintained breakdown by logical
// primitive (entities vs. FSM vs. DXP, say, all live inside the same
// "primary" group server-side). Re-deriving that finer split here
// would mean hand-maintaining a table-name-to-primitive mapping
// outside xolu's own source — precisely the kind of
// independently-drifting classification this endpoint exists to
// avoid.
func summaryFindings(s *xclient.TenantSummary) []Finding {
	findings := []Finding{
		mapFinding("primary", s.Primary),
		mapFinding("loc", s.Loc),
		mapFinding("obj", s.Obj),
		scalarFinding("ts", s.TS),
		scalarFinding("cal_index", s.CalIndex),
		scalarFinding("bal_rollup", s.BalRollup),
		scalarFinding("blob", s.Blob),
	}
	return findings
}

func mapFinding(name string, counts map[string]int) Finding {
	var nonZero []string
	for table, n := range counts {
		if n > 0 {
			nonZero = append(nonZero, fmt.Sprintf("%s:%d", table, n))
		}
	}
	if len(nonZero) == 0 {
		return Finding{Primitive: name, Status: FindingEmpty}
	}
	sort.Strings(nonZero)
	return Finding{Primitive: name, Status: FindingHasData, Detail: fmt.Sprintf("%v", nonZero)}
}

func scalarFinding(name string, n int) Finding {
	if n == 0 {
		return Finding{Primitive: name, Status: FindingEmpty}
	}
	return Finding{Primitive: name, Status: FindingHasData, Detail: fmt.Sprintf("%d", n)}
}
