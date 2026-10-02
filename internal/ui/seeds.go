// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// seeds.go — HTTP handlers for the seed system: browse local seed
// packages, preview one (manual, images, a computed step summary),
// apply it to a connected tenant (gated by the emptiness check unless
// explicitly skipped), and roll back a failed or unwanted apply.
package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	xclient "github.com/ha1tch/xolu/pkg/client"

	mi "github.com/ha1tch/minty"
	"github.com/ha1tch/xoluman/internal/config"
	"github.com/ha1tch/xoluman/internal/connstore"
	"github.com/ha1tch/xoluman/internal/modules"
	"github.com/ha1tch/xoluman/internal/seedapply"
	"github.com/ha1tch/xoluman/internal/seeds"
	"github.com/ha1tch/xoluman/internal/seedsremote"
	"github.com/ha1tch/xoluman/internal/xoluext"
)

// RegisterSeedsModule registers the seed system's routes onto reg.
func RegisterSeedsModule(reg *modules.Registry, store connstore.Store) {
	h := &seedsHandler{store: store}
	reg.Register(modules.Module{
		ID: "seeds",
		MountRoutes: func(mux *http.ServeMux) {
			mux.HandleFunc("GET /connections/{name}/seeds", h.Browse)
			mux.HandleFunc("POST /connections/{name}/seeds/sync-remote", h.SyncRemote)
			mux.HandleFunc("GET /connections/{name}/seeds/{id}", h.Preview)
			mux.HandleFunc("GET /connections/{name}/seeds/{id}/preview/{file...}", h.PreviewImage)
			mux.HandleFunc("POST /connections/{name}/seeds/{id}/apply", h.ApplySeed)
			mux.HandleFunc("POST /connections/{name}/seeds/rollback", h.RollbackSeed)
		},
	})
}

type seedsHandler struct {
	store connstore.Store
}

// clientFor resolves name to a real xclient.Client, writing the
// standard not-found/upstream-error response and returning a nil
// client when resolution fails — callers check for a nil client
// rather than duplicating this error handling themselves.
func (h *seedsHandler) clientFor(w http.ResponseWriter, r *http.Request, name string) *xclient.Client {
	conn, err := h.store.Get(r.Context(), name)
	if err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return nil
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return nil
	}
	return xoluext.BuildClient(conn)
}

// remoteCacheDir is where a successful remote sync's own content
// lands — xoluman's own config directory, not the local SeedsDir,
// since the two sources are never conflated: a local seed a person
// configured themselves and a remote one fetched from
// ha1tch/xoluseeds are shown, and identified, separately (see the
// "remote-" id prefix used throughout this file).
func remoteCacheDir() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "remote-seeds-cache"), nil
}

// remoteDiscoverResult is what a Browse page load's own remote check
// found — Synced distinguishes "sync failed, nothing to show but an
// error and a retry button" from "sync succeeded, zero seeds found"
// (today's real state of the still-empty ha1tch/xoluseeds), which
// discoverSeeds' own plain *seeds.DiscoverResult can't express on its
// own.
type remoteDiscoverResult struct {
	Attempted bool // false when SeedAllowRemoteSources is off entirely
	SyncErr   error
	Seeds     []*seeds.LoadedSeed
}

// discoverRemoteSeeds performs the seed system's own "check one time"
// remote step — one Sync attempt, no retry loop of any kind here;
// retrying is exclusively the manual /seeds/sync-remote endpoint,
// triggered only by a person clicking the retry button this function's
// own failure path renders. tarballURL is an explicit parameter
// (callers pass seedsremote.DefaultZipURL) rather than hardcoded
// here, so tests can point it at a fake server instead of the real
// network.
func discoverRemoteSeeds(ctx context.Context, settings config.Settings, tarballURL string) remoteDiscoverResult {
	if !settings.SeedAllowRemoteSources {
		return remoteDiscoverResult{}
	}
	cacheDir, err := remoteCacheDir()
	if err != nil {
		return remoteDiscoverResult{Attempted: true, SyncErr: err}
	}
	if err := seedsremote.Sync(ctx, tarballURL, cacheDir); err != nil {
		return remoteDiscoverResult{Attempted: true, SyncErr: err}
	}
	result, err := seeds.Discover(cacheDir)
	if err != nil {
		return remoteDiscoverResult{Attempted: true, SyncErr: err}
	}
	return remoteDiscoverResult{Attempted: true, Seeds: result.Seeds}
}

const remoteIDPrefix = "remote-"

// discoverSeeds loads xoluman's own configured seeds directory and
// discovers every seed package in it. An unconfigured SeedsDir (the
// default, first-run state) is not an error — an empty
// seeds.DiscoverResult, same as Discover's own treatment of a
// directory that doesn't exist yet.
func discoverSeeds() (*seeds.DiscoverResult, error) {
	settings, err := config.Load()
	if err != nil {
		return nil, err
	}
	if settings.SeedsDir == "" {
		return &seeds.DiscoverResult{}, nil
	}
	return seeds.Discover(settings.SeedsDir)
}

// findSeed resolves id to a loaded seed — a plain id looks up a local
// seed (re-discovering fresh, so an edit on disk is picked up on the
// very next request); a "remote-" prefixed id looks up a seed
// previously placed in the remote cache by the most recent successful
// Browse-page sync. findSeed never itself triggers a new remote sync
// — that only ever happens from Browse or the explicit retry
// endpoint, matching the "check once" design.
func findSeed(id string) (*seeds.LoadedSeed, error) {
	if strings.HasPrefix(id, remoteIDPrefix) {
		cacheDir, err := remoteCacheDir()
		if err != nil {
			return nil, err
		}
		result, err := seeds.Discover(cacheDir)
		if err != nil {
			return nil, err
		}
		want := strings.TrimPrefix(id, remoteIDPrefix)
		for _, s := range result.Seeds {
			if s.Manifest.ID == want {
				return s, nil
			}
		}
		return nil, fmt.Errorf("no remote seed with id %q — it may need re-syncing from Browse", want)
	}
	result, err := discoverSeeds()
	if err != nil {
		return nil, err
	}
	for _, s := range result.Seeds {
		if s.Manifest.ID == id {
			return s, nil
		}
	}
	return nil, fmt.Errorf("no seed with id %q", id)
}

// Browse lists every local seed package available to apply, plus,
// when enabled, the remote source — one check per page load, never
// more.
func (h *seedsHandler) Browse(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := h.store.Get(r.Context(), name); err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	settings, err := config.Load()
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}
	result, err := discoverSeeds()
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}
	remote := discoverRemoteSeeds(r.Context(), settings, seedsremote.DefaultZipURL)

	body := func(b *mi.Builder) mi.Node {
		header := b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white mb-4"), "Seeds for "+name)

		localSection := renderSeedSection(b, name, "", result.Seeds, result.Failures)
		var remoteSection mi.Node
		if remote.Attempted {
			remoteSection = renderRemoteSection(b, name, remote)
		}

		if len(result.Seeds) == 0 && len(result.Failures) == 0 && !remote.Attempted {
			return b.Div(header,
				b.P(mi.Class("text-gray-500 dark:text-gray-400"), "No seeds configured. Set a seeds directory in Settings to browse packaged data sets you can apply here."),
			)
		}

		return b.Div(header, localSection, remoteSection)
	}
	WriteHTML(w, Page("Seeds — "+name, r.URL.Path, body))
}

// renderSeedSection renders one list of loaded seeds plus any load
// failures — shared between the local list and the remote list, which
// differ only in id prefix (idPrefix) and framing.
func renderSeedSection(b *mi.Builder, connName, idPrefix string, loaded []*seeds.LoadedSeed, failures []seeds.DiscoverFailure) mi.Node {
	if len(loaded) == 0 && len(failures) == 0 {
		return nil
	}
	var cards []interface{}
	sorted := append([]*seeds.LoadedSeed{}, loaded...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Manifest.Name < sorted[j].Manifest.Name })
	for _, s := range sorted {
		href := "/connections/" + url.PathEscape(connName) + "/seeds/" + url.PathEscape(idPrefix+s.Manifest.ID)
		cards = append(cards, b.Div(mi.Class("border rounded-lg p-4 mb-3 dark:border-gray-700"),
			b.A(mi.Href(href), mi.Class("text-lg font-medium text-indigo-600 dark:text-indigo-400"), s.Manifest.Name),
			b.P(mi.Class("text-sm text-gray-600 dark:text-gray-400 mt-1"), s.Manifest.Description),
		))
	}

	var failureNodes []interface{}
	for _, f := range failures {
		failureNodes = append(failureNodes, b.Div(mi.Class("text-sm text-red-600 dark:text-red-400"), f.Dir+": "+f.Err.Error()))
	}
	var failuresBlock mi.Node
	if len(failureNodes) > 0 {
		failuresBlock = b.Div(mi.Class("mt-2 p-3 border border-red-200 dark:border-red-900 rounded-lg"),
			b.P(mi.Class("text-sm font-medium text-red-700 dark:text-red-400 mb-1"), fmt.Sprintf("%d seed(s) failed to load:", len(failureNodes))),
			b.Div(failureNodes...),
		)
	}
	return b.Div(mi.Class("mb-4"), b.Div(cards...), failuresBlock)
}

// renderRemoteSection shows the remote source's own section: the
// seeds found on success, or the sync error plus a manual retry
// button on failure — the retry button is the ONLY way this ever
// tries again; there is no automatic retry anywhere in this flow.
func renderRemoteSection(b *mi.Builder, connName string, remote remoteDiscoverResult) mi.Node {
	heading := b.P(mi.Class("text-sm font-medium text-gray-500 dark:text-gray-400 mb-2 mt-6"), "From ha1tch/xoluseeds")

	if remote.SyncErr != nil {
		errBlock := mi.Raw(`
			<div class="p-3 border border-red-200 dark:border-red-900 rounded-lg">
				<p class="text-sm text-red-600 dark:text-red-400 mb-2">Could not reach the remote seed source: ` + htmlEscape(remote.SyncErr.Error()) + `</p>
				<button class="` + btnSecondary + `" onclick="retrySeedSync()">Retry</button>
			</div>
			<script>
			async function retrySeedSync() {
				await fetch('/connections/` + url.PathEscape(connName) + `/seeds/sync-remote', { method: 'POST' });
				location.reload();
			}
			</script>
		`)
		return b.Div(heading, errBlock)
	}

	if len(remote.Seeds) == 0 {
		return b.Div(heading, b.P(mi.Class("text-sm text-gray-500 dark:text-gray-400"), "No seeds available yet."))
	}

	return b.Div(heading, renderSeedSection(b, connName, remoteIDPrefix, remote.Seeds, nil))
}

// SyncRemote is the seed system's own manual retry — the only place
// a remote sync is ever attempted outside a Browse page load. Refuses
// outright, doing no network access at all, if the setting is off —
// this endpoint existing at all is not itself permission to sync; the
// setting is checked every time, not just at Browse.
func (h *seedsHandler) SyncRemote(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := h.store.Get(r.Context(), name); err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}
	settings, err := config.Load()
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if !settings.SeedAllowRemoteSources {
		_ = json.NewEncoder(w).Encode(map[string]any{"attempted": false})
		return
	}
	remote := discoverRemoteSeeds(r.Context(), settings, seedsremote.DefaultZipURL)
	resp := map[string]any{"attempted": true, "seedCount": len(remote.Seeds)}
	if remote.SyncErr != nil {
		resp["error"] = remote.SyncErr.Error()
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// Preview shows one seed's own metadata, rendered manual, preview
// images, and a computed per-step-type summary of what applying it
// would create — everything a person needs to decide before ever
// touching the connected tenant.
func (h *seedsHandler) Preview(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")
	if _, err := h.store.Get(r.Context(), name); err != nil {
		if errors.Is(err, connstore.ErrNotFound) {
			writeConnectionNotFound(w, name)
			return
		}
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	ls, err := findSeed(id)
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}
	manual, err := ls.ReadManual()
	if err != nil {
		writeUpstreamError(w, name, r.URL.Path, err)
		return
	}

	basePath := "/connections/" + url.PathEscape(name) + "/seeds/" + url.PathEscape(id)

	body := func(b *mi.Builder) mi.Node {
		header := b.Div(mi.Class("mb-4"),
			b.H1(mi.Class("text-xl font-semibold text-gray-900 dark:text-white"), ls.Manifest.Name),
			b.P(mi.Class("text-gray-600 dark:text-gray-400 mt-1"), ls.Manifest.Description),
		)

		var imageNodes []interface{}
		for _, img := range ls.Manifest.PreviewImages {
			src := basePath + "/preview/" + img
			imageNodes = append(imageNodes, b.Img(mi.Src(src), mi.Class("rounded-lg border dark:border-gray-700 max-w-xs mr-2 mb-2")))
		}
		var gallery mi.Node
		if len(imageNodes) > 0 {
			gallery = b.Div(append([]interface{}{mi.Class("flex flex-wrap mb-4")}, imageNodes...)...)
		}

		var manualBlock mi.Node
		if manual != "" {
			manualBlock = b.Div(mi.Class("prose dark:prose-invert max-w-none mb-4"), b.Pre(mi.Class("whitespace-pre-wrap text-sm"), manual))
		}

		summary := stepSummary(ls.Manifest.Steps)
		var summaryRows []interface{}
		for _, row := range summary {
			summaryRows = append(summaryRows, b.Div(mi.Class("flex justify-between text-sm py-1 border-b dark:border-gray-700"),
				b.Span(row.label), b.Span(fmt.Sprintf("%d", row.count)),
			))
		}
		summaryBlock := b.Div(mi.Class("border rounded-lg p-4 mb-4 dark:border-gray-700"),
			b.P(mi.Class("font-medium text-gray-900 dark:text-white mb-2"), "This seed will create:"),
			b.Div(summaryRows...),
		)

		applyForm := mi.Raw(`
			<div id="seed-apply-area">
				<button id="seed-apply-btn" class="` + btnPrimary + `" onclick="applySeed()">Apply this seed</button>
				<div id="seed-apply-result" class="mt-3"></div>
			</div>
			<script>
			async function applySeed() {
				var btn = document.getElementById('seed-apply-btn');
				var out = document.getElementById('seed-apply-result');
				btn.disabled = true;
				btn.textContent = 'Applying…';
				out.innerHTML = '';
				try {
					var resp = await fetch('` + basePath + `/apply', { method: 'POST' });
					var data = await resp.json();
					if (data.blocked) {
						out.innerHTML = '<div class="text-red-600 dark:text-red-400 text-sm">Blocked: the connection is not empty. ' +
							(data.emptinessCheck && data.emptinessCheck.findings ? JSON.stringify(data.emptinessCheck.findings) : '') + '</div>';
					} else if (data.success) {
						out.innerHTML = '<div class="text-green-600 dark:text-green-400 text-sm">Applied successfully — ' + data.created.length + ' item(s) created.</div>';
					} else {
						var rollbackBtn = '<button class="` + btnSecondary + `" style="margin-top:0.5rem" onclick=\'rollbackSeed(' + JSON.stringify(data.created) + ')\'>Roll back what was created</button>';
						out.innerHTML = '<div class="text-red-600 dark:text-red-400 text-sm">Failed at step ' + data.failedAtStep + ': ' + data.error + '</div>' + rollbackBtn;
					}
				} finally {
					btn.disabled = false;
					btn.textContent = 'Apply this seed';
				}
			}
			async function rollbackSeed(created) {
				var out = document.getElementById('seed-apply-result');
				var resp = await fetch('/connections/` + url.PathEscape(name) + `/seeds/rollback', {
					method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({created: created})
				});
				var data = await resp.json();
				out.innerHTML = '<div class="text-sm">Removed ' + (data.removed ? data.removed.length : 0) + ' item(s). ' +
					(data.notDeletable && data.notDeletable.length ? data.notDeletable.length + ' could not be removed (no delete capability exists for that kind).' : '') + '</div>';
			}
			</script>
		`)

		return b.Div(header, gallery, manualBlock, summaryBlock, applyForm)
	}
	WriteHTML(w, Page(ls.Manifest.Name, r.URL.Path, body))
}

type stepSummaryRow struct {
	label string
	count int
}

// stepSummary aggregates a manifest's own steps into per-kind counts
// for the preview page — a person deciding whether to apply a seed
// needs "3 entity types, 1 FSM, 2 DXP defs," not a raw step list.
func stepSummary(stepsList []seeds.Step) []stepSummaryRow {
	counts := map[seeds.StepType]int{}
	for _, s := range stepsList {
		counts[s.Type]++
	}
	labels := map[seeds.StepType]string{
		seeds.StepSchema:            "Entity schemas",
		seeds.StepData:              "Entity data files",
		seeds.StepFSM:               "FSM definitions",
		seeds.StepDXPDef:            "DXP definitions",
		seeds.StepBalDefine:         "bal accounts",
		seeds.StepBalTransfer:       "bal transfers",
		seeds.StepCalCreateCalendar: "cal calendars",
		seeds.StepCalPropose:        "cal bookings",
		seeds.StepRaw:               "Other (raw) steps",
	}
	// Stable order matches the label table above, not map iteration.
	order := []seeds.StepType{seeds.StepSchema, seeds.StepData, seeds.StepFSM, seeds.StepDXPDef, seeds.StepBalDefine, seeds.StepBalTransfer, seeds.StepCalCreateCalendar, seeds.StepCalPropose, seeds.StepRaw}
	var rows []stepSummaryRow
	for _, t := range order {
		if counts[t] > 0 {
			rows = append(rows, stepSummaryRow{label: labels[t], count: counts[t]})
		}
	}
	return rows
}

// PreviewImage serves one file from a seed's own directory — a
// preview image or the manual, referenced by the manifest's own
// relative paths. http.FileServer/http.Dir handle path-traversal
// safety directly (a cleaned, rooted path; "../" cannot escape the
// seed's own directory), not hand-rolled here.
func (h *seedsHandler) PreviewImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ls, err := findSeed(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file := r.PathValue("file")
	http.ServeFile(w, r, ls.Dir+"/"+file)
}

// applyResponse is ApplySeed's own JSON response shape.
type applyResponse struct {
	Blocked        bool                       `json:"blocked,omitempty"`
	EmptinessCheck *seedapply.EmptinessResult `json:"emptinessCheck,omitempty"`
	Success        bool                       `json:"success"`
	Error          string                     `json:"error,omitempty"`
	// FailedAtStep deliberately has no omitempty — 0 is a real,
	// meaningful step index (the first step), not an absent value.
	// omitempty on an int field drops it from the JSON entirely when
	// it's the zero value, which would silently turn a genuine
	// "failed at step 0" into a missing key — caught directly by
	// running the real UI flow against a real server, where the
	// preview page's own JS read `data.failedAtStep` as `undefined`
	// for exactly this failure.
	FailedAtStep int                 `json:"failedAtStep"`
	Created      []seedapply.Created `json:"created"`
}

// ApplySeed applies one seed to the connected tenant — gated by the
// emptiness check (internal/seedapply.CheckEmpty) unless the person
// has explicitly opted out via Settings.SeedSkipEmptyCheck. Created is
// always present in the response, even on a partial failure, since
// the preview page's own rollback button needs exactly that list.
func (h *seedsHandler) ApplySeed(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")
	c := h.clientFor(w, r, name)
	if c == nil {
		return
	}
	ls, err := findSeed(id)
	if err != nil {
		writeJSONError(w, err, name)
		return
	}
	settings, err := config.Load()
	if err != nil {
		writeJSONError(w, err, name)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if !settings.SeedSkipEmptyCheck {
		emptiness := seedapply.CheckEmpty(r.Context(), c)
		if !emptiness.Empty {
			_ = json.NewEncoder(w).Encode(applyResponse{Blocked: true, EmptinessCheck: emptiness, Created: []seedapply.Created{}})
			return
		}
	}

	result := seedapply.Apply(r.Context(), c, ls)
	resp := applyResponse{Created: result.Created}
	if result.Created == nil {
		resp.Created = []seedapply.Created{}
	}
	if result.Err != nil {
		resp.Success = false
		resp.Error = result.Err.Error()
		resp.FailedAtStep = result.FailedAtStep
	} else {
		resp.Success = true
	}
	_ = json.NewEncoder(w).Encode(resp)
}

type rollbackRequest struct {
	Created []seedapply.Created `json:"created"`
}

// RollbackSeed undoes as much of a prior apply as xolu's own client
// allows — see seedapply.Rollback's own doc comment for exactly what
// that does and doesn't cover.
func (h *seedsHandler) RollbackSeed(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c := h.clientFor(w, r, name)
	if c == nil {
		return
	}
	var req rollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	report := seedapply.Rollback(r.Context(), c, req.Created)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report)
}
