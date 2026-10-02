// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package seedapply

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	xclient "github.com/ha1tch/xolu/pkg/client"
)

// emptyTenantSummaryBody matches the real response shape confirmed
// live against a genuinely fresh xolu v0.30.34 tenant.
func emptyTenantSummaryBody() map[string]any {
	return map[string]any{
		"primary":    map[string]int{},
		"loc":        map[string]int{},
		"obj":        map[string]int{},
		"ts":         0,
		"cal_index":  0,
		"bal_rollup": 0,
		"blob":       0,
		"empty":      true,
	}
}

func serveTenantSummary(t *testing.T, body map[string]any) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func TestCheckEmpty_GenuinelyEmptyTenant(t *testing.T) {
	c := xclient.New(serveTenantSummary(t, emptyTenantSummaryBody()))
	result := CheckEmpty(context.Background(), c)
	if !result.Empty {
		t.Fatalf("Empty = false, want true. Findings: %+v", result.Findings)
	}
	if len(result.Findings) != 7 {
		t.Fatalf("got %d findings, want 7 (one per top-level group), findings: %+v", len(result.Findings), result.Findings)
	}
	for _, f := range result.Findings {
		if f.Status != FindingEmpty {
			t.Errorf("finding %q status = %q, want empty", f.Primitive, f.Status)
		}
	}
}

func TestCheckEmpty_PrimaryHasData(t *testing.T) {
	body := emptyTenantSummaryBody()
	body["primary"] = map[string]int{"nodes": 5, "edges": 0}
	body["empty"] = false
	c := xclient.New(serveTenantSummary(t, body))

	result := CheckEmpty(context.Background(), c)
	if result.Empty {
		t.Fatal("Empty = true, want false -- primary has real data")
	}
	var primaryFinding *Finding
	for i := range result.Findings {
		if result.Findings[i].Primitive == "primary" {
			primaryFinding = &result.Findings[i]
		}
	}
	if primaryFinding == nil || primaryFinding.Status != FindingHasData {
		t.Fatalf("primary finding = %+v, want FindingHasData", primaryFinding)
	}
	if primaryFinding.Detail != "[nodes:5]" {
		t.Errorf("primary detail = %q, want it to name the nonzero table (edges:0 must be excluded, only nonzero tables listed)", primaryFinding.Detail)
	}
}

func TestCheckEmpty_ObjHasData(t *testing.T) {
	// obj was permanently uncheckable in the previous implementation
	// (no server-side enumeration endpoint existed at all) -- this
	// confirms it's now a real, working check via the new
	// tenant-summary endpoint, not still a gap.
	body := emptyTenantSummaryBody()
	body["obj"] = map[string]int{"obj_subjects": 3}
	body["empty"] = false
	c := xclient.New(serveTenantSummary(t, body))

	result := CheckEmpty(context.Background(), c)
	if result.Empty {
		t.Fatal("Empty = true, want false -- obj has real data")
	}
	var objFinding *Finding
	for i := range result.Findings {
		if result.Findings[i].Primitive == "obj" {
			objFinding = &result.Findings[i]
		}
	}
	if objFinding == nil || objFinding.Status != FindingHasData {
		t.Fatalf("obj finding = %+v, want FindingHasData -- obj must no longer be an unconditional gap", objFinding)
	}
}

func TestCheckEmpty_ScalarFieldHasData(t *testing.T) {
	body := emptyTenantSummaryBody()
	body["blob"] = 12
	body["empty"] = false
	c := xclient.New(serveTenantSummary(t, body))

	result := CheckEmpty(context.Background(), c)
	if result.Empty {
		t.Fatal("Empty = true, want false -- blob has real data")
	}
	var blobFinding *Finding
	for i := range result.Findings {
		if result.Findings[i].Primitive == "blob" {
			blobFinding = &result.Findings[i]
		}
	}
	if blobFinding == nil || blobFinding.Status != FindingHasData || blobFinding.Detail != "12" {
		t.Fatalf("blob finding = %+v, want FindingHasData with detail \"12\"", blobFinding)
	}
}

func TestCheckEmpty_RequestFailureIsNeverTreatedAsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"deliberate failure for this test"}`))
	}))
	t.Cleanup(server.Close)
	c := xclient.New(server.URL)

	result := CheckEmpty(context.Background(), c)
	if result.Empty {
		t.Fatal("Empty = true, want false -- a failed tenant-summary call must never be treated as confirmed-empty")
	}
	if len(result.Findings) != 1 || result.Findings[0].Status != FindingCheckFailed {
		t.Fatalf("Findings = %+v, want exactly one FindingCheckFailed", result.Findings)
	}
}

func TestCheckEmpty_AllSevenGroupsAlwaysReported(t *testing.T) {
	c := xclient.New(serveTenantSummary(t, emptyTenantSummaryBody()))
	result := CheckEmpty(context.Background(), c)
	want := map[string]bool{"primary": false, "loc": false, "obj": false, "ts": false, "cal_index": false, "bal_rollup": false, "blob": false}
	for _, f := range result.Findings {
		want[f.Primitive] = true
	}
	for group, seen := range want {
		if !seen {
			t.Errorf("group %q missing from Findings", group)
		}
	}
}
