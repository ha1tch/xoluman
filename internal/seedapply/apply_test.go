// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package seedapply

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/seeds"
)

// requestLog records every request the fake server received, in
// order, for tests that need to assert on exactly what was sent —
// most importantly the resolved REF value in a phase-B patch.
type requestLog struct {
	mu       sync.Mutex
	requests []loggedRequest
}

type loggedRequest struct {
	Method string
	Path   string
	Body   map[string]any
}

func (l *requestLog) record(r *http.Request) map[string]any {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	l.mu.Lock()
	l.requests = append(l.requests, loggedRequest{Method: r.Method, Path: r.URL.Path, Body: body})
	l.mu.Unlock()
	return body
}

func (l *requestLog) find(method, path string) (loggedRequest, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.requests {
		if r.Method == method && r.Path == path {
			return r, true
		}
	}
	return loggedRequest{}, false
}

// fakeXolu builds an httptest server covering every real endpoint
// Apply's own step executors hit — paths and response shapes
// confirmed directly against xolu's real client source (buildURLv2's
// own construction, each method's own doc comment), not guessed.
// entityIDs assigns sequential ids per entity type on Create, mirroring
// a real server's own autoincrement behavior closely enough for these
// tests' own purposes.
func fakeXolu(t *testing.T) (*httptest.Server, *requestLog) {
	t.Helper()
	log := &requestLog{}
	nextID := map[string]int64{}
	var mu sync.Mutex

	mux := http.NewServeMux()

	// Entity create/patch (v1, unscoped since no tenant context is set
	// in these tests -- see buildURL's own "" tenantID branch).
	mux.HandleFunc("POST /api/v1/{entity}", func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		entity := r.PathValue("entity")
		mu.Lock()
		nextID[entity]++
		id := nextID[entity]
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
	})
	mux.HandleFunc("PATCH /api/v1/{entity}/{id}", func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
	})

	// Schema (root, /api/v1 prefix per buildURLRoot -- not tenant-scoped).
	mux.HandleFunc("POST /api/v1/schema/{entity}", func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "ok"})
	})

	// FSM def (v2).
	mux.HandleFunc("POST /api/v2/fsm/def", func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 501, "name": "test-fsm", "created_at": "2026-01-01T00:00:00Z"})
	})

	// DXP def (v2).
	mux.HandleFunc("POST /api/v2/dxp/def", func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 601, "name": "test-dxp"})
	})

	// bal (v2).
	mux.HandleFunc("POST /api/v2/bal/def", func(w http.ResponseWriter, r *http.Request) {
		body := log.record(r)
		_ = json.NewEncoder(w).Encode(map[string]any{"account_id": body["account_id"], "unit": body["unit"]})
	})
	mux.HandleFunc("POST /api/v2/bal/transfer", func(w http.ResponseWriter, r *http.Request) {
		body := log.record(r)
		_ = json.NewEncoder(w).Encode(map[string]any{"transfer_id": "t-generated", "from": body["from"], "to": body["to"]})
	})

	// cal (v2).
	mux.HandleFunc("POST /api/v2/cal/calendars", func(w http.ResponseWriter, r *http.Request) {
		body := log.record(r)
		_ = json.NewEncoder(w).Encode(map[string]any{"calendar_id": body["calendar_id"]})
	})
	mux.HandleFunc("POST /api/v2/cal/propose", func(w http.ResponseWriter, r *http.Request) {
		body := log.record(r)
		_ = json.NewEncoder(w).Encode(map[string]any{"booking_id": body["booking_id"], "calendar_id": body["calendar_id"], "state": "proposed"})
	})

	// raw catch-all -- ts events, or anything else a raw step targets.
	mux.HandleFunc("POST /ts/events/batch", func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		w.WriteHeader(http.StatusCreated)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, log
}

// loadSeedFromFiles builds a real, on-disk seed directory from a
// manifest plus a set of step files, then loads it — the same path a
// real seed would go through, not a shortcut in-memory construction.
func loadSeedFromFiles(t *testing.T, manifestJSON string, files map[string]string) *seeds.LoadedSeed {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "seed.json"), []byte(manifestJSON), 0644); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("writing %s: %v", rel, err)
		}
	}
	ls, err := seeds.Load(dir)
	if err != nil {
		t.Fatalf("loading seed: %v", err)
	}
	return ls
}

func TestApply_EveryStepType(t *testing.T) {
	server, _ := fakeXolu(t)
	c := xclient.New(server.URL)

	manifest := `{
		"format_version": 1, "id": "full", "name": "Full",
		"steps": [
			{"type":"schema","entity_type":"companies","file":"schemas/companies.json"},
			{"type":"data","entity_type":"companies","file":"data/companies.jsonl"},
			{"type":"fsm","file":"fsms/a.json"},
			{"type":"dxp_def","file":"dxp/a.json"},
			{"type":"bal_define","file":"bal/define.json"},
			{"type":"bal_transfer","file":"bal/transfer.json"},
			{"type":"cal_create_calendar","file":"cal/create.json"},
			{"type":"cal_propose","file":"cal/propose.json"},
			{"type":"raw","method":"POST","path":"/ts/events/batch","file":"raw/ts.json"}
		]
	}`
	files := map[string]string{
		"schemas/companies.json": `{"type":"object","properties":{"name":{"type":"string"}}}`,
		"data/companies.jsonl":   `{"_key":"acme","name":"Acme"}`,
		"fsms/a.json":            `{"name":"approval","initial":"start","states":{"start":{}},"transitions":[]}`,
		"dxp/a.json":             `{"name":"close-deal","pattern":"saga","participants":[{"id":"p1","primitive":"entity","op":"patch","params":{}}],"phase_ttl":{"reserve":"30s"}}`,
		"bal/define.json":        `{"account_id":"acct-1","unit":"USD","scale":2}`,
		"bal/transfer.json":      `{"from":"acct-1","to":"acct-2","amount":"10.00","scale":2}`,
		"cal/create.json":        `{"calendar_id":"cal-1"}`,
		"cal/propose.json":       `{"booking_id":"bk-1","calendar_id":"cal-1","span":{"start":"2026-01-01T00:00:00Z","end":"2026-01-01T01:00:00Z"}}`,
		"raw/ts.json":            `{"events":[{"timeline":1,"dims":[1],"time":"2026-01-01T00:00:00Z","nums":[1.5]}]}`,
	}
	ls := loadSeedFromFiles(t, manifest, files)

	result := Apply(context.Background(), c, ls)
	if result.Err != nil {
		t.Fatalf("unexpected error at step %d: %v", result.FailedAtStep, result.Err)
	}
	if len(result.Created) != 9 {
		t.Fatalf("got %d Created records, want 9 (one per step): %+v", len(result.Created), result.Created)
	}
	kinds := map[CreatedKind]int{}
	for _, c := range result.Created {
		kinds[c.Kind]++
	}
	wantKinds := []CreatedKind{CreatedSchema, CreatedEntity, CreatedFSMDef, CreatedDXPDef, CreatedBalAccount, CreatedBalTransfer, CreatedCalCalendar, CreatedCalBooking, CreatedRaw}
	for _, k := range wantKinds {
		if kinds[k] != 1 {
			t.Errorf("kind %q: got %d, want 1", k, kinds[k])
		}
	}
}

func TestApply_RefResolutionAcrossSteps(t *testing.T) {
	server, log := fakeXolu(t)
	c := xclient.New(server.URL)

	// The referencing entity (deals) is declared in an earlier step
	// than the entity it references (users) -- a forward reference,
	// confirming order genuinely doesn't matter.
	manifest := `{
		"format_version": 1, "id": "refs", "name": "Refs",
		"steps": [
			{"type":"data","entity_type":"deals","file":"data/deals.jsonl"},
			{"type":"data","entity_type":"users","file":"data/users.jsonl"}
		]
	}`
	files := map[string]string{
		"data/deals.jsonl": `{"_key":"deal1","name":"Big Deal","owner":{"$ref":"alice"}}`,
		"data/users.jsonl": `{"_key":"alice","name":"Alice"}`,
	}
	ls := loadSeedFromFiles(t, manifest, files)

	result := Apply(context.Background(), c, ls)
	if result.Err != nil {
		t.Fatalf("unexpected error at step %d: %v", result.FailedAtStep, result.Err)
	}

	patchReq, found := log.find(http.MethodPatch, "/api/v1/deals/1")
	if !found {
		t.Fatal("expected a PATCH to /api/v1/deals/1 resolving the deferred owner ref, none found")
	}
	owner, ok := patchReq.Body["owner"].(map[string]any)
	if !ok {
		t.Fatalf("patch body owner = %v, want a resolved REF object", patchReq.Body["owner"])
	}
	if owner["entity"] != "users" || owner["type"] != "REF" {
		t.Errorf("resolved ref = %+v, want entity=users type=REF", owner)
	}
	// alice was the first (and only) user created -> id 1.
	if id, _ := owner["id"].(float64); id != 1 {
		t.Errorf("resolved ref id = %v, want 1", owner["id"])
	}
}

func TestApply_StopsAtFirstFailure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/{entity}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
	})
	mux.HandleFunc("POST /api/v2/fsm/def", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"deliberate failure for this test"}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := xclient.New(server.URL)

	manifest := `{
		"format_version": 1, "id": "fails", "name": "Fails",
		"steps": [
			{"type":"data","entity_type":"companies","file":"data/companies.jsonl"},
			{"type":"fsm","file":"fsms/a.json"},
			{"type":"data","entity_type":"contacts","file":"data/contacts.jsonl"}
		]
	}`
	files := map[string]string{
		"data/companies.jsonl": `{"name":"Acme"}`,
		"fsms/a.json":          `{"name":"x"}`,
		"data/contacts.jsonl":  `{"name":"Should never be created"}`,
	}
	ls := loadSeedFromFiles(t, manifest, files)

	result := Apply(context.Background(), c, ls)
	if result.Err == nil {
		t.Fatal("expected an error, got none")
	}
	if result.FailedAtStep != 1 {
		t.Errorf("FailedAtStep = %d, want 1 (the fsm step)", result.FailedAtStep)
	}
	// The companies entity from step 0 succeeded before the failure --
	// it must still be in Created for Rollback to find it.
	if len(result.Created) != 1 || result.Created[0].Kind != CreatedEntity {
		t.Errorf("Created = %+v, want exactly the one entity created before the failure", result.Created)
	}
}

func TestApply_UndefinedRefKeyFails(t *testing.T) {
	server, _ := fakeXolu(t)
	c := xclient.New(server.URL)

	manifest := `{
		"format_version": 1, "id": "badref", "name": "BadRef",
		"steps": [
			{"type":"data","entity_type":"deals","file":"data/deals.jsonl"}
		]
	}`
	files := map[string]string{
		"data/deals.jsonl": `{"name":"Big Deal","owner":{"$ref":"nobody-declared-this-key"}}`,
	}
	ls := loadSeedFromFiles(t, manifest, files)

	result := Apply(context.Background(), c, ls)
	if result.Err == nil {
		t.Fatal("expected an error for a $ref to an undefined key, got none")
	}
	if !strings.Contains(result.Err.Error(), "nobody-declared-this-key") {
		t.Errorf("error %q does not name the undefined key", result.Err.Error())
	}
}

func TestApply_ReadStepFileFailureReportsCorrectStepIndex(t *testing.T) {
	// A manifest step whose file existed at Load time but vanished
	// before Apply ran (or any other read failure) must still report
	// the correct step index, not silently misattribute it.
	server, _ := fakeXolu(t)
	c := xclient.New(server.URL)
	ls := loadSeedFromFiles(t,
		`{"format_version":1,"id":"x","name":"X","steps":[{"type":"fsm","file":"fsms/a.json"}]}`,
		map[string]string{"fsms/a.json": `{"name":"x"}`},
	)
	// Remove the file after Load succeeded, before Apply runs.
	if err := os.Remove(filepath.Join(ls.Dir, "fsms/a.json")); err != nil {
		t.Fatalf("removing file: %v", err)
	}
	result := Apply(context.Background(), c, ls)
	if result.Err == nil {
		t.Fatal("expected an error, got none")
	}
	if result.FailedAtStep != 0 {
		t.Errorf("FailedAtStep = %d, want 0", result.FailedAtStep)
	}
}
