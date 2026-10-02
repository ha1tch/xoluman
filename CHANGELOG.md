# Changelog

All notable changes to xoluman are recorded here.

## [0.7.32] — 2026-08-19

FSM export — SVG, PNG, and LaTeX/TikZ — reusing `fsm-toolkit`'s own
exporters rather than xoluman inventing anything: the unreused
capability flagged directly, and the gap that motivated porting
LaTeX/TikZ to Go in the first place, now actually wired in.

- **`fsm-toolkit` bumped to v0.10.0** (from v0.9.6, via a local
  replace directive — that version isn't published anywhere yet),
  bringing `pkg/latex` into scope alongside the `pkg/fsmfile` SVG/PNG
  exporters xoluman's `fsmdef.go` already had access to but never
  called.
- **Three new routes**: `GET .../fsm/{id}/export/{svg,png,latex}`.
  All three share the existing `fsmToolkitFSM` conversion bridge
  (already used for local validation) rather than duplicating it.
- **One deliberate, stated difference between formats, not papered
  over**: SVG and PNG run `fsm-toolkit`'s own `SmartLayout`
  internally and always have — a fresh auto-layout every export.
  LaTeX runs no layout of its own by design (see `pkg/latex`'s own
  package doc comment) — it renders the actual arrangement saved via
  the existing `SaveLayout` mechanism. A machine with no saved layout
  yet gets a clear, actionable error for the LaTeX path specifically,
  not a fabricated position set that wouldn't match what's on screen.
- **`fsm-editor.js`'s own toolbar already had SVG / PNG / TeX export
  buttons wired to a `_exportBaseUrl` getter derived from `layoutUrl`**
  — found already present and already correctly pointing at these
  exact route paths when this file was checked, not written in this
  pass. Confirmed the derivation is genuinely correct against the real
  server-rendered `layout-url` attribute rather than assumed.
- **Two real integration bugs found and fixed by actually running the
  tests, not caught by review**: `RenderPNG` needs real width/height
  — a zero-value `PNGOptions{}` produces `invalid image size: 0x0`;
  fixed to use `fsm-toolkit`'s own `DefaultSVGOptions()`/
  `DefaultPNGOptions()`, which already existed and should have been
  used from the start. Separately, a test's own mock of the
  `xoluman_fsm_layout` list response used a guessed shape
  (`{"entities": [...]}`) instead of the real one
  (`{"data": [...], "pagination": {...}}`) — caught by reading
  `xolu`'s own `Client.List` implementation directly rather than
  continuing to guess.
- **Verified live, end to end**, not only via unit tests: a real FSM
  created and a real layout saved through xoluman's own actual HTTP
  API, then all three export endpoints hit for real. SVG and PNG
  visually confirmed correct (PNG's auto-layout genuinely
  auto-derived, distinct green initial-state styling from
  `fsm-toolkit` itself). The LaTeX output was compiled with `pdflatex`
  for real and the resulting diagram visually confirmed to match the
  *exact* saved coordinates, not an auto-derived approximation of
  them — the specific distinction this format exists to preserve.

## [0.7.31] — 2026-08-19

The xolu team delivered on every gap flagged in the seed-system
requirements report — verified independently before anything here
was built on top of it, not accepted on the strength of their own
changelog.

- **`xolu` dependency bumped to v0.30.34** (from v0.30.14). Before
  touching xoluman's own code, every claim was checked directly: the
  real route table for `obj`'s new `GET /obj/list` and all 12 typed
  client methods, `loc`'s full 21-method coverage including fences
  and patterns, `ts`'s buildout from 7 to 31 routes, and the two
  specific fixes called out in their changelog (`LocPatchRequest`'s
  double-pointer tri-state semantics; `obj`'s `TxnID`/
  `CommittedThrough` as `int64`/`int`, not `string`) — all confirmed
  exactly as described by reading the actual source, not the prose
  describing it.
- **`internal/seedapply.CheckEmpty` rewritten around the new,
  single-round-trip `Client.TenantSummary`**, replacing the previous
  eight-primitive, multiple-round-trip implementation entirely. This
  closes the one permanent gap that implementation could never
  actually close on its own — `obj` had no enumeration endpoint
  anywhere in xolu's own server, confirmed directly at the time, so
  no client-side workaround could ever make it checkable — and adds
  `blob` coverage that the earlier implementation had simply never
  attempted at all, a real gap in xoluman's own prior coverage, not
  only xolu's. `FindingUncheckable` is gone; every group is now a
  real, working check.
- **Verified live, end to end, through the real HTTP API**, not just
  unit tests: registered a connection against a freshly-launched
  `xolu` v0.30.34 server, applied a real seed against a genuinely
  empty tenant (correctly allowed, FSM genuinely created), then
  applied again against the now-populated tenant — correctly blocked,
  with an accurate breakdown naming exactly what was found
  (`primary: [fsm_definitions:1 fsm_id_seq:1]`, every other group
  correctly reported empty).
- **One stale test caught and fixed along the way**: a test asserting
  the blocked-on-non-empty path mocked the old, now-unused
  `/api/v1/entities` route directly. With no `/tenant-summary` route
  in that same mock, the new `CheckEmpty` would have 404'd and still
  reported blocked — passing, but for an unrelated reason (a failed
  check, not genuine data detection) rather than the one it claimed
  to test. Fixed to mock the real endpoint with genuine non-empty
  data, so the test verifies what it says it verifies.
- No UI or wire-format changes needed: `EmptinessResult`'s own
  `Findings []Finding` shape is unchanged, and the preview page's own
  JS already renders it generically — only `CheckEmpty`'s internal
  implementation changed.

## [0.7.30] — 2026-08-15

Switched the remote seed source from `.tar.gz` to `.zip`, per direct
feedback — no good argument survived being actually checked for
keeping `.tar.gz`.

- **The rate-limit argument doesn't favor either format.**
  `codeload.github.com` serves `.zip` as an equally single request,
  same as `.tar.gz` — the reason tar.gz was chosen in the first place
  (avoiding the GitHub REST API's per-directory calls burning through
  its unauthenticated 60/hour limit) applies just as well to `.zip`.
  `.zip`'s own simpler, non-nested container — no separate compression
  layer wrapping the archive structure, unlike gzip wrapping tar — was
  preferred once that was checked rather than defended reflexively.
- **`internal/seedsremote.Sync` rewritten around `archive/zip`.**
  `DefaultZipURL` replaces `DefaultTarballURL`
  (`codeload.github.com/ha1tch/xoluseeds/zip/refs/heads/main`). One
  real, honestly-stated tradeoff: `archive/zip` needs random access
  (its central directory sits at the end of the file), so the whole
  response is read into memory before extraction, rather than streamed
  entry-by-entry the way a tar stream allowed — a real cost only for
  archives too large to comfortably buffer, and seed packages (schemas,
  JSONL, small preview images) aren't that.
- Wrapper-directory stripping and the tar-path-traversal defense both
  carry over unchanged in spirit — same "zip slip"-class protection,
  now checking each zip entry's own resolved path instead of each tar
  header's.
- All 6 `seedsremote` tests and the `internal/ui` remote-source tests
  rewritten to build real, in-memory `.zip` fixtures instead of
  `.tar.gz` ones — same coverage (extraction, empty-repo handling,
  path-traversal rejection, stale-cache clearing, HTTP/format-error
  propagation), same assertions, different container format.
- **Re-verified live against the real repository** after the switch:
  the `.zip` endpoint extracts the exact same real content (`LICENSE`,
  `README.md`) the `.tar.gz` version did, and `Discover` again
  correctly reads it as zero seeds, not a failure.

## [0.7.29] — 2026-08-15

The remote seed source: `ha1tch/xoluseeds`, now real and public
(currently empty, per its own owner), wired end to end — checked, and
re-checked, against the actual live repository, not just mocked
fixtures.

- **New `internal/seedsremote` package.** `Sync` fetches the repo's
  own tarball (`codeload.github.com`, one request regardless of how
  many seed packages exist — deliberately not the GitHub REST API's
  own per-directory contents calls, which would burn through its
  unauthenticated 60/hour rate limit after a handful of seeds, and
  would turn "one check" into an unbounded request count as the repo
  grows) and extracts it into a local cache directory, stripping
  GitHub's own top-level wrapper directory so the result lands exactly
  where `internal/seeds.Discover` already expects it — no separate
  "remote seed" data model exists anywhere; a synced remote seed is
  discovered, loaded, previewed, and applied through precisely the
  same code path as a local one.
- **Tar path-traversal safety treated as a real concern, not
  theoretical** — a tarball is untrusted input the moment it comes
  from a network fetch. Every extracted entry's resolved path is
  confirmed to stay within the cache directory before being written;
  a deliberately malicious tar entry (`../../../etc/evil.json`) is
  rejected outright, confirmed with a dedicated test that also checks
  the file was never actually written anywhere on disk, not just that
  an error came back.
- **Wired into `Browse` and a new manual-retry endpoint
  (`POST .../seeds/sync-remote`)**, matching the agreed design
  exactly: one sync attempt per page load when the setting is on, zero
  network access at all when it's off (confirmed with a test pointed
  at a guaranteed-unreachable address), and retrying is *only* ever
  the person clicking the retry button this renders on failure — no
  background polling, no automatic retry, anywhere.
- Local and remote seeds are shown, and addressed, separately — a
  `remote-` id prefix in the URL disambiguates a remote seed from a
  local one that happens to share the same author-chosen id, resolved
  in `findSeed` without ever triggering a new sync of its own (sync
  only ever happens from `Browse` or the explicit retry).
- 13 new tests across both packages (tarball extraction, the empty-repo
  case, path-traversal rejection, stale-cache clearing between syncs,
  HTTP/gzip error propagation, the disabled-setting no-network-access
  guarantee, remote-id resolution, and the retry endpoint's own refusal
  when the setting is off).
- **Verified live, twice**: a hand-built local check confirming `Sync`
  against the real repository extracts exactly its real current
  content (`LICENSE`, `README.md`) and that `Discover` correctly reads
  this as zero seeds, not a failure; then the full browser-driven
  flow — Browse page, real network round trip to
  `codeload.github.com`, "No seeds available yet" rendered cleanly,
  zero console errors.

## [0.7.28] — 2026-08-15

The seed system's HTTP layer and settings — browse, preview, apply,
and roll back a seed against a connected tenant, reachable through
real routes for the first time. Two real bugs found only by running
the whole thing live against a real xolu server, not just the mocked
unit tests — recorded in full since both are worth remembering.

- **New settings in `internal/config`**: `SeedsDir`,
  `SeedSkipEmptyCheck`, `SeedAllowRemoteSources`. Both booleans are
  deliberately oriented so their zero value is the safe one — a
  `settings.json` written before these fields existed decodes to
  "check runs, remote sources off" automatically, never to an
  accidental opt-out. A `RequireEmptyConnection: true`-by-default
  field would have failed exactly this case, silently, since JSON
  unmarshal leaves an absent field at its zero value, not at whatever
  `DefaultSettings` would have produced — proved with a dedicated test
  that decodes a genuinely pre-existing settings file and asserts both
  fields land safe.
- **New `internal/ui/seeds.go`**: browse (list local seeds), preview
  (manual, image gallery, a computed step-count summary), apply
  (gated by the emptiness check unless explicitly skipped), and
  rollback — registered as a real module alongside every other one.
  Preview images served via `http.FileServer`/`http.Dir`, so
  path-traversal safety comes from Go's own established mechanism, not
  hand-rolled.
- **Bug 1, `FailedAtStep` silently dropped on the wire.** `omitempty`
  on an `int` field where 0 is a real, meaningful value (the first
  step) — Go's JSON encoding treats the zero value as absent and omits
  the key entirely. A failure at step 0 showed up in the browser as
  "Failed at step *undefined*." Fixed by removing `omitempty` from
  this one field specifically.
- **Bug 2, the more interesting one: `Rollback`'s own report types had
  no JSON tags at all.** They serialized with Go's default,
  capitalized field names (`"Removed"`, `"NotDeletable"`) while the
  UI's own JS read lowercase/camelCase (`data.removed`), so every
  rollback silently reported "Removed 0 item(s)" regardless of what
  actually happened — confirmed via a raw `curl` call that the backend
  itself was correct throughout (the real server showed the item
  genuinely removed) while the browser display was wrong the entire
  time. A second, related gap in the same area: `RollbackFailure.Err`
  is a Go `error`, which has no exported fields at all — the default
  encoding would have silently produced `{}`, losing any real failure
  message on the wire. Fixed with proper `json:"camelCase"` tags plus
  a custom `MarshalJSON` on `RollbackFailure` that surfaces
  `Err.Error()` as a real string field. Three new tests assert the
  actual serialized JSON keys directly — the exact gap that let both
  bugs through undetected until a live run caught them.
- **Verified against a real xolu server, twice over, after each fix**:
  a real seed applied successfully (an FSM genuinely created,
  confirmed via a direct API call, not just a UI success message);
  and a deliberately-failing two-step seed, rolled back, confirmed
  genuinely removed on the real server afterward — both runs with zero
  browser console errors.

## [0.7.27] — 2026-08-15

The seed system's safety precondition: `CheckEmpty` in
`internal/seedapply`, confirming a connection has no data at all
before a seed is ever allowed to apply (per the agreed default: the
switch is on unless explicitly turned off, and when it's on, nothing
gets written unless the tenant is perfectly empty).

- **Checks across every primitive with a real way to check**:
  entities (`ListEntities`, any type with `Count > 0` fails it), FSM
  defs, DXP defs, `bal` accounts, `cal` calendars, `ts` timelines (all
  via their own real, typed `List` methods), and `loc` (no typed
  client method exists, so via `Client.Raw` against the real `GET
  /loc/list` route — confirmed directly against
  `v2_loc_handlers.go`'s own route table and response shape, not
  guessed).
- **`obj` is permanently uncheckable, stated outright rather than
  worked around.** Checked xolu's real `v2_obj_handlers.go` directly:
  every registered route is a per-id lookup (get, move, report,
  position, contents) — no enumeration endpoint exists anywhere in the
  route table. A real design tension followed from this: requiring
  every primitive to be *confirmed* empty would mean the safety switch
  could never pass for any connection, ever, which would make the
  switch block the feature entirely rather than serve it. Resolved by
  giving `obj` its own status, `FindingUncheckable`, which — alone
  among the failure statuses — does not by itself make the overall
  result non-empty, but is always included in the report, never
  silently dropped, so the gap in the "perfectly empty" guarantee is
  visible to whoever is deciding whether to proceed, not hidden from
  them.
- A genuine failed check (a check that errors, times out, or returns
  something unexpected) is treated the same as "has data" for the
  overall verdict — "couldn't verify" is deliberately never conflated
  with "confirmed empty."
- Two real path mistakes caught by running the tests against a real
  mock server, not assumed correct from the method names alone: `ts`'s
  real list route is `/ts/tl/list`, not the more obvious-sounding
  guess this started with; and this session's own test mux needed
  restructuring after Go 1.22+'s `ServeMux` turned out to panic on
  registering the same route pattern twice, which the first draft's
  override style did without noticing.
- 7 new tests (all-empty, each primitive individually reporting real
  data, a failed check, and two tests asserting the `obj` design
  decision explicitly by name, not just incidentally). Full suite,
  both new packages plus everything existing, clean.

## [0.7.26] — 2026-08-15

The seed system's executor: `internal/seedapply`, everything needed
to actually run a loaded seed against a real, connected tenant.

- **Two-phase `Apply`.** Phase one runs every step in manifest order —
  each of the nine step kinds decodes straight into its own real
  `xclient` request type and calls the real method (`schema` →
  `DefineEntitySchema`, `data` → `Create` per row with REF fields
  deferred, `fsm` → `CreateMachineDef`, `dxp_def` → `DxpDefCreate`,
  `bal_define`/`bal_transfer` → `BalDefine`/`BalTransfer`,
  `cal_create_calendar`/`cal_propose` → `CalCreateCalendar`/
  `CalPropose`, `raw` → `Raw`). Phase two patches every deferred `$ref`
  field back in, once every seed-local key in the *whole* seed has a
  real target — order never matters, forward references across steps
  confirmed working directly, not assumed.
- **`Rollback`, honest about its own real limits.** Checked `xolu`'s
  public client directly for delete/cancel/remove methods before
  writing this: only entities and FSM definitions can ever be removed
  through it. Nothing exists for DXP defs, `bal` accounts/transfers,
  or `cal` calendars/bookings — confirmed by absence, not assumed from
  "probably immutable." `Rollback` never pretends otherwise: those
  kinds are reported in `NotDeletable`, never attempted. This is the
  concrete reason the seed system's own empty-connection precondition
  (agreed several turns ago) matters as much as it does — for the
  non-deletable kinds, refusing to start against a non-empty tenant is
  the *only* real safety net that exists.
- **Two real bugs caught by actually running the tests against a real
  mock server**, not assumed away: the schema-registration mock route
  was missing xolu's real `/api/v1` prefix (`buildURLRoot`'s own
  construction, checked directly); a DXP-def test fixture was
  genuinely invalid per `xolu`'s own client-side validation (empty
  `participants`, then a missing `phase_ttl.reserve`) — both traced to
  the real cause and fixed, not patched around.
- **A format clarification this work surfaced**: `Client.Raw` does no
  path prefixing at all (confirmed against its real implementation —
  a bare `baseURL + path` concatenation), so a `raw` step's own `path`
  must be the complete, absolute path, not a short suffix. Documented
  directly on `seeds.Step`'s own field.
- 17 new tests (every step kind in one full-manifest run, cross-step
  forward-reference resolution, mid-run failure with `Created`
  correctly populated up to the failure point for `Rollback`'s own
  input, an undefined-`$ref`-key failure, a vanished-file failure) —
  all passing, full existing suite unaffected.

## [0.7.25] — 2026-08-15

First real implementation piece of the seed system: the manifest
format itself, built and tested in isolation before anything else
depends on it, same pattern as `oqlclassify`.

- **New `internal/seeds` package.** `Manifest`/`Step` types plus a
  pure, filesystem-free `ParseManifest` — deliberately: unit-testable
  without a real directory on disk, filesystem/existence checks are a
  separate, later concern (`Load`, not yet built).
- **Nine step kinds**, five typed and four not, matching exactly what
  was confirmed against xolu's real source in the requirements report:
  `schema`, `data`, `fsm`, `dxp_def`, `bal_define`, `bal_transfer`,
  `cal_create_calendar`, `cal_propose` (typed — each will decode
  straight into `xolu`'s own real client request struct once the
  executor exists) and `raw` (a plain `{method, path, file}`
  passthrough — the only route available today for `loc`/`obj`/`ts`,
  all three confirmed to have real, working REST endpoints despite no
  client-library wrapper; `ts` specifically via `POST
  .../ts/events/batch`, exact field shape confirmed against the real,
  unexported server handler type).
- Structural validation at parse time, not deferred to the executor:
  unknown `format_version`, unknown step type, missing `id`/`name`,
  zero steps, missing `file`, `schema`/`data` steps missing
  `entity_type`, `raw` steps missing `method`/`path` — all rejected
  immediately with an error naming the manifest's own id, not
  discovered later as a confusing runtime failure.
- 16 tests, one per validation rule plus the full/minimal valid-shape
  cases, all passing on the first run. Full existing suite unaffected.

## [0.7.24] — 2026-08-15

- **Added `github.com/ha1tch/queryfy@v0.3.2`** as a direct dependency
  — the newest release, not just matching whatever version `xolu`
  itself happens to depend on transitively (`v0.3.1`, confirmed by
  checking `xolu`'s own `go.mod` directly). Landed as `// indirect`
  in `go.mod`, correctly: nothing in `xoluman`'s own code imports it
  yet, so Go's own tooling is being honest about that rather than
  claiming a use that doesn't exist — the marker will drop on its own
  the moment real code does, most likely the seed-format validation
  work discussed but not yet built. Full build, vet, and test suite
  confirmed clean with the addition in place.

## [0.7.23] — 2026-08-15

Completes the query/graph view reorganization design: Sulpher moves
out of the Query view into a new tab beside the graph walker, with a
"View in graph" bridge for graph-shaped results. The Query view keeps
just OQL and REST.

- **`classifySulpherResult` extracted** from the dormant standalone
  endpoint into a shared, reused function — confirmed the extraction
  changed nothing (the existing dormant tests still pass unchanged).
  `Run`'s own `sulpher` case now returns the same classified
  `{nodes, edges}` alongside the raw result (`sulpherRunResponse`,
  mirroring `oqlRunResponse`'s own embedding pattern), so the Sulpher
  tab can offer "View in graph" without a second round trip.
- **`query-editor.js` gained a `modes` attribute** restricting which
  tabs an instance shows — `oql,rest` on the Query page, `sulpher`
  alone on the Graph page. The tab switcher itself is hidden entirely
  when only one mode is active. A new `_viewInGraph()` dispatches a
  bubbling custom event carrying the classified graph data;
  `fsm-editor.js` gained a public `applyExternalGraphData()` entry
  point as the receiving end.
- **New combined `graphPage()`** wrapper loading both script sets
  (fsm-canvas-engine.js + fsm-editor.js, codemirror-bundle +
  query-editor.js) and a small page-level tab controller mediating
  between the two sibling components — neither needs to know the
  other exists. Graph view is the default, initially-visible tab
  deliberately: `fsm-editor.js`'s own initial sizing reads the
  canvas-wrap's real bounding box at connect time, which would see
  0×0 if that panel started hidden; `query-editor.js` has no
  equivalent concern, so Sulpher starting hidden is safe the other
  way around.
- **Found and root-caused a real bug in the Cypher editor
  integration**, not previously exercised by a real run-and-render
  cycle: any Sulpher query threw `newContentVersion is not a
  function` from a debounced lint timer — confirmed this fired from
  typing alone, with no Run click at all, ruling out anything specific
  to this turn's own changes. Traced through the actual, unminified
  `@neo4j-cypher/codemirror@1.0.3` source (installed and read
  directly, not guessed from the minified vendored bundle) to the real
  cause: the package's own lower-level `getExtensions()` API — what
  `query-editor.js` uses, for the same `Compartment`-based
  reconfiguration OQL/REST already need — never attaches
  `newContentVersion` to the `EditorView` itself; only the package's
  separate, higher-level `createCypherEditor()` helper does that,
  as a manually-attached version-counter closure. Fixed by completing
  that same attachment directly in `query-editor.js`, matching the
  package's own internal implementation exactly — no bundle rebuild,
  no feature loss (linting and autocomplete both still fully work).
- Verified end to end against real, live CRM data throughout: the new
  tab structure, a real Sulpher query with zero errors through typing,
  waiting, and running, the full bridge to the graph view (4 real
  nodes, all 4 correct entity types), and confirmed both the
  OQL/REST-only Query page and FSM-mode pages are genuinely unaffected
  by any of this turn's changes.

## [0.7.22] — 2026-08-14

Two graph-editor UX fixes, requested directly: a full-screen toggle,
and moving the inspect panel from below the canvas to a right sidebar
(a preference from an earlier session that the actual layout hadn't
matched — the panel was rendering as a sibling `<div>` below the
canvas-wrap, not beside it, since the outer container is a
`flex-direction: column` layout and nothing had ever put the two side
by side).

- **Right sidebar for graph mode's inspect panel.** New
  `.fsm-graph-body` row-flex wrapper (canvas + a fixed-width, 320px
  `.fsm-graph-sidebar`) used only in graph mode — FSM mode's own
  layout (validation panel below the canvas) is untouched, confirmed
  directly rather than assumed: canvas stays full-width, no sidebar
  element present at all. Confirmed against real, live data with an
  actually-populated inspect panel, not just the empty "click a node"
  hint state, since that hint text lacked the `.fsm-panel` class the
  first draft of this fix would have relied on.
- **Full-screen toggle**, a new `⛶` button beside the existing zoom
  controls. Deliberately a CSS-only `position: fixed` overlay covering
  the viewport rather than the real browser Fullscreen API — avoids
  the permission prompt, the "press Esc to exit" browser chrome, and
  needing to listen for `fullscreenchange` separately to keep state in
  sync; toggling back is just flipping the same reactive property.
  Re-fits the view when toggled (`zoomToFit`, after waiting for the
  actual resize to complete) — the existing `ResizeObserver`-driven
  resize already correctly grows the canvas buffer, but never
  auto-refits on its own, which is right for an ordinary window resize
  but wrong here: full screen exists to see more of the graph, not
  more empty canvas at the same zoom level. Available in both graph
  and FSM mode, not scoped narrowly — the same convenience applies
  equally well to FSM editing, confirmed present in both. Verified
  against real, live data: canvas genuinely grows on maximize and
  restores to its exact original size on exit.

## [0.7.21] — 2026-08-14

Wires the OQL classifier (v0.7.20) into the actual query view — the
second piece of the query/graph reorganization design. REST stays
untouched (no table-view routing at all, per this design's own
correction: REST is a raw passthrough, not meant to simulate OQL).

- **`Run`'s OQL response now carries classification.** `oqlRunResponse`
  embeds `*xclient.OQLResult` by pointer so `status`/`data`/`stats`
  keep flattening to the top level exactly as before this change —
  nothing that already read the old shape needs to change — plus a
  new `classification: {isSimpleSelect, sourceTable}` field, computed
  via `oqlclassify.Classify` against the actual query text. A query
  the classifier itself can't parse degrades to `isSimpleSelect:
  false` rather than failing the whole request — the real OQL result
  a person asked for is never blocked by a gap in this session's own
  classifier.
- **New UI in `query-editor.js`, OQL mode only:** a generic "View as
  table" toggle for any result whose own `data` is a non-empty array
  (client-side, built directly from the rows already in hand — no new
  endpoint needed), and, only when the classifier confirms a genuine
  simple select, a second link straight to the real, existing,
  already-live-editable entity list page for that table
  (`/connections/{name}/entities/{table}`) — not a new editable view
  built for this feature, the one that was already there.
- Verified end to end against real, live CRM data: `SELECT * FROM
  companies` correctly shows both the toggle and the link (link
  pointing at the real entities page, toggle rendering a genuine
  9-column, 15-row table); `SELECT name, industry FROM companies`
  correctly shows the toggle alone, no link; REST mode shows neither,
  confirmed directly rather than assumed from the `this._mode ===
  'oql'` guard alone; Sulpher mode confirmed still present and
  unaffected.
- One test-script mistake caught and fixed along the way, not an
  application bug: an initial verification script assumed the mode
  tabs were `<button>` elements: they're plain `<div class="xolu-
  query-tab">`s, confirmed directly against the actual render output
  before fixing the selector.

## [0.7.20] — 2026-08-14

First piece of the query/graph view reorganization design (Sulpher
moving into the graph view as its own tab; OQL results routing to
either the existing live-editable entity grid or a new read-only
table view, depending on query shape) — built and tested in isolation
first, since it's the part carrying the most real risk.

- **New `internal/oqlclassify` package.** Determines whether an OQL
  query is a "simple select" — `SELECT * FROM <one table>`, nothing
  else — the one shape where every returned row is, one-to-one, a
  complete and unmodified document of a real entity, safe to route
  into the existing entity grid for live editing. Every other shape
  (an explicit field list even if it happens to name every field, a
  JOIN, an aggregate, `GROUP BY`, `UNION`, `DISTINCT`, `SELECT INTO`,
  a subquery in `FROM`) is correctly rejected — confirmed against
  `tsqlparser`'s own real, parsed AST rather than text matching,
  which would have gotten real OQL syntax wrong somewhere (comments,
  bracketed identifiers, whitespace) for a decision where a false
  positive is the one failure mode that actually matters (routing an
  uneditable result into a live-edit grid).
- **New dependency: `github.com/ha1tch/tsqlparser`**, the same T-SQL
  AST library xolu's own OQL planner already depends on for
  equivalent push-down decisions (`PushJoin`/`PushAggregate`/
  `PushFull` in xolu's own `pkg/oql/planner.go`) — reused rather than
  duplicated with a separate, weaker heuristic.
- Every design assumption verified empirically against the real
  parser before being relied on, not trusted from the struct
  definitions alone: confirmed a JOIN's two sides collapse into a
  single `FromClause.Tables` entry (a different concrete AST type,
  not two slice entries — a naive `len(Tables) == 1` check would have
  wrongly classified joins as simple), and confirmed `COUNT(*)`'s
  `*` lives inside the function's own argument, never the top-level
  `SelectColumn.AllColumns` flag Go's own struct field would suggest
  needed a separate check. Both findings changed the actual
  implementation before it shipped, not just the tests.
- 17 tests, covering every disqualifying shape and, separately, every
  shape that must *not* disqualify (`WHERE`, `ORDER BY`, `TOP`,
  `OFFSET`/`FETCH` narrow, order, or limit rows without changing what
  each row is) — all passing on the first run once the AST behavior
  was confirmed empirically.
- Not yet wired to anything visible — this is the classifier alone,
  built and verified standalone before any UI work depends on it.

## [0.7.19] — 2026-08-12

The graph-editing design's core, long-standing goal, completed:
creating a real edge by dragging from one node's halo to another,
with both the schema-ful and schema-less rules this session's design
conversation established.

- **"Clear relationship"** — right-click an edge, replacing the old
  FSM-mode "Transition Properties" dialog that had no meaning against
  xolu's own relationship model (a REF value on a field, not a first-
  class object with its own properties). `SaveGraphEdge` extended:
  an explicitly empty `newTo` clears the field rather than retargets
  it — confirmed directly first that `PATCH` with a null value
  genuinely removes the field server-side, not just stores a literal
  null. `showFSMDialog` gained a configurable `saveLabel` and a fix
  for an empty `fields` array (would have thrown) to support this as
  a plain confirm-style dialog rather than a form.
- **Edge creation via drag** — new `RefFieldsForType` endpoint
  (with `hasSchema`, added after catching a real gap of my own before
  it shipped: distinguishing a genuinely schema-less type from a
  schema-ful one with no currently-usable field, since only the
  former gets the permissive "any target, ask for a field name"
  treatment). `onmousedown`/`onmouseup` wrapped in graph mode: left-
  click-halo now starts a real drag (previously suppressed entirely)
  and completes by calling the origin's own REF-field info, finding
  the first empty field whose known target matches the drop (schema-
  ful), or prompting for a field name when the origin's type has no
  schema at all (schema-less) — then persists via the existing
  `SaveGraphEdge`, same as retargeting already does.
- **A long, genuinely instructive debugging arc getting the drag to
  actually complete in a real browser.** `currentLink` kept coming
  back `null` mid-drag no matter how the mouse movement was shaped
  (single jump, ten-step interpolation, twenty small manual steps —
  none of it was the cause). Traced it to a real, confirmed
  `onmouseleave` firing mid-drag; traced that to the actual root
  cause: the halo-click logic sets `selectedObject` to the drag's own
  origin as a side effect of how it already works for FSM mode, and
  the graph viewer's own selection-change logic reacted to that by
  opening the inspect panel *mid-drag* — confirmed directly, the
  canvas shrank by roughly 130px to make room for it, pulling the
  still-held mouse position outside the new, smaller bounds. Not a
  test artifact: a real person dragging in a real browser would hit
  the identical disruption. Fixed by suppressing the inspect-panel
  reaction for the drag's own duration.
- Verified end to end against real, live CRM data for both paths:
  schema-less (real drag between two freshly-created schema-less
  entities, real field-name prompt, `related_to` genuinely persisted,
  `_version` incremented) and schema-ful (a remembered target seeded
  first, since the real CRM schema declares none; real drag from a
  task with a genuinely empty `contact` field to a real contact node;
  the field automatically matched with no prompt at all, since the
  target was already known — confirmed persisted via a direct API
  check independent of the UI). FSM mode's own halo behavior
  re-confirmed unaffected.

## [0.7.18] — 2026-08-12

New-node creation in the graph viewer, end to end: double-click empty
canvas, right-click to assign an entity type, fill in and submit the
real entity form, node becomes a genuinely persisted xolu entity.

- **Go**: `resolveEntityFields` converted from a method to a free
  function (it never used its own receiver — mechanical, low-risk,
  only 2 call sites). New `CreateGraphNode` endpoint, deliberately not
  a reuse of `entities.go`'s own `Create` — that one redirects on
  success, which has no new ID anywhere to read back. Built from the
  same underlying pieces (`resolveEntityFields`,
  `refTargetsForSubmission`, `formengine.ParseFormValues`,
  `rememberNewRefTargets`) with a JSON response shaped for an AJAX
  caller instead. 5 new tests; found one genuine mistake in a test of
  my own (a form key omitted entirely rather than sent present-but-
  empty, not how a real `<input>` element submits) — fixed the test,
  confirmed the handler's own validation logic was correct as written.
- **JS**: `showFSMDialog` gained an optional `select` field type
  (entity-type picking from real, known types — no free-text typo
  risk). New `showHTMLDialog`, a separate dialog for injecting
  arbitrary HTML — specifically the real, unmodified `NewForm`
  fragment, fetched via the `HX-Request` mechanism `WriteModalAware`
  already understood (no new fragment endpoint needed at all — found
  already built while investigating). `ondblclick` and `oncontextmenu`
  wrapped for graph mode, matching the halo-interception pattern from
  last session: double-click marks a new node `_graphDraft`, colours
  it `invalid`; right-click on a draft node starts the type-assignment
  flow; on success the node is upgraded in place (real `_graphNode`,
  colour cleared) using a follow-up `Expand` call at depth 0 for the
  real, server-computed data rather than approximating it from raw
  submitted strings.
- **A long, genuinely instructive debugging detour**: end-to-end
  testing initially looked like a hung promise inside the new dialog
  chain — spent a long stretch isolating it (bare functions, methods
  on the real component instance, direct source dumps, line-by-line
  logging) before finding it wasn't a hang or a bug in the application
  at all. Two separate mistakes in the test scripts themselves: a bare
  `'select'` locator matched the graph toolbar's own pre-existing
  entity-type dropdown instead of the dialog's new one (the page
  legitimately has two `<select>` elements); and an assumption that
  the `role` field renders as a dropdown, when `resolveEntityFields`
  actually renders it as plain text. Fixed both, then confirmed the
  full flow genuinely works — real double-click, real right-click,
  real form fill, real submit, node correctly transitions from a red
  draft to a real, coloured-default node, confirmed persisted via a
  direct API check independent of the UI.

## [0.7.17] — 2026-08-12

Foundational piece of a larger, still-in-progress graph-editing design
(node/edge creation, entity-type assignment, schema-aware REF
constraints — design agreed, implementation continuing next).

- **Removed the free, user-initiated colour-cycling gesture entirely**
  (Shift+click on a link) — confirmed it was reachable, ungated by
  mode, in graph mode too (same class of issue as the halo and
  context-menu bugs fixed last turn). It cycled through four colours
  with no meaning attached to any of them — a demonstration of the
  mechanism, never a real feature. Removed from the shared canvas
  engine entirely, not just gated out of graph mode, since the same
  reasoning applies wherever it's used. Confirmed FSM mode's own
  stationary-click label-editing (restructured from the same
  `if`/`else if` chain) is unaffected.
- **New shared, semantic colour palette** (`seamStateColors`) — colour
  now only ever reflects real, operative state, assigned
  programmatically, never picked or cycled by a person. Two states so
  far (`unsaved`, `invalid`), deliberately not exhaustive; more can be
  added without touching any drawing code. The same palette backs both
  nodes and edges, so the same colour value means the same thing
  regardless of which object type it's applied to — no separate,
  divergent colour vocabularies.
- **`nodeColors`**, the node equivalent of the existing `linkColors`
  map, wired into the actual node-drawing pass (mirroring exactly how
  link colour overrides already worked) — plus `ensureNodeId`, nodes'
  own counterpart to `ensureLinkId`, needed since nodes previously had
  no stable identity of their own to key a colour map by. Verified the
  override genuinely renders, not just that the JS state is set
  correctly — sampled actual canvas pixels via `getImageData` before
  and after setting a node's colour, confirmed a real, visible shift
  from the default indigo stroke toward the assigned semantic red.

## [0.7.16] — 2026-08-12

- **Collapsed nodes in the graph viewer**, addressing the boundary
  where depth (or the fetch ceiling, or an unreachable target) runs
  out: previously the edge to that target was recorded but the target
  itself was silently absent, so the JS side dropped the edge
  entirely (nothing to draw it to). Now the target gets a real,
  visible node — collapsed, minimal data — reusing
  `fsm-canvas-engine.js`'s own double-circle terminal/accept-state
  style (`isAcceptState`) rather than inventing a new visual.
  - Server: `graphNode` gained `Collapsed`; the walker
    (`internal/ui/graphrest.go`) adds a collapsed stub instead of
    dropping an unfetched target, and correctly upgrades a stub to a
    full node in place if a different path later reaches it with hop
    budget to spare — confirmed with a dedicated test, not just
    assumed correct from the logic.
  - New `POST .../graph/expand` endpoint (and `restEmbedGraphRunner.
    Expand`) fetches one specific collapsed node's real data plus one
    hop of its own REFs, reusing the same walker.
  - JS: clicking a collapsed node shows a distinct panel ("Collapsed
    node: type#id") with an Expand action, rather than the normal
    (and here pointless, given a stub has no real data) edit form.
    Expanding merges the result into the live canvas in place —
    upgrading an existing stub rather than duplicating it, skipping
    edges already present — deliberately not a full re-query, which
    would reset everyone's pan/zoom/position for no reason.
  - **Collapse**, the explicit inverse: purely client-side, no server
    call needed, since it only hides data already on the canvas.
    Removes a node's own outgoing edges, then prunes any neighbor left
    with nothing else touching it — a neighbor still needed by some
    other, still-visible edge correctly stays. Confirmed against real,
    densely-connected CRM data (25 deals sharing a small pool of
    owners): expand and collapse round-tripped node/edge counts back
    to their exact starting values.
  - **Gesture: right-click on the halo** (the existing hover affordance
    fsm-canvas-engine.js already shows near a node) toggles expand/
    collapse — the fast path alongside the inspect panel's own button.
    Left-click-halo is suppressed in graph mode entirely: there's no
    xoluman feature yet for "create a new, real REF relationship" for
    it to mean. Both were previously ungated by mode at all — checked
    directly what they did in graph mode before this, and both were
    actively broken (left: a fake, unpersisted `Link` silently created
    between two real entities; right: a brand-new `Node` with no real
    data behind it at all).
  - **Found and fixed a second, unrelated, genuinely pre-existing bug**
    while sanity-checking that FSM mode's own halo behavior was
    unaffected: `canvas.onmouseup`'s `add-linked-node` drop-on-empty-
    canvas path used `mouse.x`/`mouse.y` without the function ever
    declaring a local `mouse` variable at all — every use of this FSM-
    mode feature (right-click-halo, drag to empty space, release) threw
    "mouse is not defined" and silently did nothing. Unrelated to
    today's own changes (`onmouseup` was never touched by them), found
    only because testing this turn's own work happened to exercise it
    for the first time this session. Fixed.

## [0.7.15] — 2026-08-12

Prompted by a real bug report ("saved queries don't work, FSMs don't
save, the graph doesn't persist, REFs should be edges") — every part
investigated and fixed or rebuilt, each confirmed against real,
running code, not assumed from reading the code alone.

- **Fixed the actual root cause of "saved queries don't work" and
  "FSMs don't save": two independent, stacked bugs.**
  - `query-editor.js` never declared its own internal UI state
    (`_showSaveForm`, `_result`, `_savedByMode`, etc.) as reactive Lit
    properties — a plain property assignment updated the value but
    never triggered a re-render, so clicking "Save…" silently did
    nothing visible at all. Same missing-reactivity bug found and
    fixed in `dxp-editor.js`, `grid-editor.js`, and two fields in
    `fsm-editor.js` (`_name`/`_description`, later also
    `_determinism`/`_graphQuery`) while auditing every component in
    the same way.
  - Underneath that: `ListSavedQueries` and the FSM layout save/load
    logic both asked xolu for `Limit: 1000` "to get everything in one
    call." It doesn't — xolu's own documented ceiling is 100 rows per
    page (`docs/API_REFERENCE.md`); anything above that silently falls
    back to the server's configured default (10), with no error. This
    was xoluman's own misuse of a documented contract, not an xolu
    bug — confirmed directly by checking the docs before concluding
    otherwise. Fixed with `xoluext.ListAll`, a helper that properly
    follows the pagination envelope; the same bug (and fix) found in
    three more places while searching for it systematically: the
    blob-folder browser and the field-metadata loader.
- **Fixed a real, confirmed REF-integrity gap in the grid editor.**
  Editing any REF-typed column failed outright (`XOLU-VL001`) — the
  cell editor sent a bare raw ID, xolu correctly rejected it. Fixed
  with the same REF-reconstruction the entity form already had, plus
  a compound `"entityType:id"` fallback (remembered for next time)
  for the common case where a REF field's schema declares no explicit
  target — confirmed this is the common case directly against this
  session's own CRM schema, not assumed.
- **Fixed a real, confirmed bug in the list view's REF display.** REF
  fields were unconditionally excluded from the glance columns (their
  JSON Schema `Type` is `"object"`, swept up by the same exclusion
  rule as genuine object/array fields), and where a REF's target
  wasn't known, cells leaked raw Go map formatting straight into the
  page (`map[entity:companies id:13 type:REF]`). Fixed: REF fields
  included, a readable label shown even without a known target,
  clickable link only when the target is genuinely known.
- **Rebuilt the graph viewer entirely on plain REST — no query
  language at all**, after digging into `pkg/oql`/`pkg/sulpher` and
  finding that the real blocker (XM-9, `xoluman-xolu-consolidated-
  report-log.md`) is a genuine xolu-side regression: documented
  whole-node `RETURN` fails against schema-adapted entities. Rather
  than wait on that, or reimplement Sulpher's own parser client-side
  (tried, then undone — reusing another system's grammar isn't "using
  the REST API"), the new implementation walks REF fields directly:
  fetch bare (`embed_depth=0` explicitly — a server-hydrated value
  never self-identifies its own entity type, confirmed directly; only
  a bare pointer reliably does), then hydrate hop by hop via
  `xoluext.GetWithEmbed`, a new client-library gap found and worked
  around the same way (`xoluext.ListWithEmbed`, wrapping `client.Raw`
  — itself xolu's own sanctioned escape hatch, built specifically for
  this). Filed as XM-10. The old Sulpher-based path is preserved, not
  deleted — `graphSulpherDormant`, its own tests retargeted rather
  than removed, ready to reactivate once XM-9 resolves.
  - New entity-type + depth picker replaces the old free-text Sulpher
    query box; a new `GET .../entity-types` endpoint feeds it.
  - Found and fixed a second, real, pre-existing bug while testing
    the rebuilt viewer end to end: the graph inspect panel's node-save
    request never actually included the entity type at all
    (`original.type` read from the entity's own domain data, which
    has no such field — the request silently omitted `type` entirely,
    and xolu correctly rejected every save). Confirmed via a real
    save attempt, not assumed from reading the code; fixed by storing
    the type explicitly, separate from the entity's own data. Edge
    retargeting was already correct — confirmed with a real,
    successful save.
  - Verified against real, live CRM data throughout: correct node/edge
    types and counts, correct dedup across multiple paths to the same
    target, correct hop-depth clamping (tested with a real 10-deep
    chain), a successful real node edit and a successful real edge
    retarget, both confirmed via independent API checks afterward.

## [0.7.14] — 2026-08-12

- **Integrated xolu v0.30.14** (up from v0.30.10). Full build, vet,
  and test suite green, no code changes needed on xoluman's side.
- **Every remaining item from the entire xoluman → xolu report
  campaign is now resolved — twelve numbered findings across two
  report rounds, all independently confirmed against real, running
  code, not taken on any changelog's word:**
  - **XM-5** (`OFFSET`/`FETCH NEXT` a silent no-op) — v0.30.13.
    Verified: the exact isolation query now returns exactly 3 rows,
    correctly starting at row 3.
  - **XM-6** (nested object fields failing on adapted-table insert) —
    v0.30.11. Verified by running the real, unmodified CRM seed
    script — `companies.address` intact, no stripped test copy — for
    the first time since this was found; completed fully, and reading
    the data back confirmed a genuine round-trip, not just a
    successful write.
  - **XM-7b** (decimal fields scaled/mangled through JOIN) — v0.30.14,
    the last item in the whole campaign. Verified: the same JOIN query
    now returns `"5000.66"`, not `33300065`. The now-resolved comment
    documenting this was removed from the "Deals with their company's
    industry" saved query.
  - **XM-8** (no way to create a calendar through the public API) —
    v0.30.12. Verified with the full, real workflow this was always
    supposed to be: create a calendar, propose a booking, confirm it,
    list both — real HTTP calls end to end, not a partial check.
- **The real CRM example seed script is fully unblocked** for the
  first time since XM-6 was found — no more temporary, address-
  stripped workaround copy needed for any verification going forward.
- `ts`, `bal`, and `cal` are now all genuinely usable through
  xoluman's own client dependency — none of the three has UI built
  for it in xoluman yet; that's real, additive follow-up work, not
  anything blocked or owed to xolu.
- Both report documents (the full consolidated log and the pending-
  only view) updated to reflect the campaign's completion and
  re-delivered — the pending-only report now states plainly that
  nothing is pending.

## [0.7.13] — 2026-08-12

- **Integrated xolu v0.30.10** (up from v0.30.8). Full build, vet, and
  test suite green, no code changes needed on xoluman's side.
- **Re-verified all five remaining pending items — all unchanged.**
  v0.30.9 closed out xolu's own internal XOT180 audit entirely (ten
  real fixes, "all four audit threads complete"), but its final
  thread was a different, unrelated workstream — general tenant-
  isolation coverage across six list-shaped endpoints
  (`handleDxpDefList`/`handleDxpTxnList`, `handleFSMMachineList`,
  `handleEventList`, `handleSeqList`/`handleGenList`,
  `handleBlobList`) — none of which overlap XM-5, XM-6, XM-7, or
  XM-8. v0.30.10 was a register bookkeeping correction only (a stale
  duplicate tracking item closed with a cross-reference), no code
  change at all. Confirmed all five directly against a fresh v0.30.10
  instance rather than assumed unchanged from the changelog's silence
  on them: `OFFSET`/`FETCH NEXT` still returns every row (XM-5),
  `companies.address` still fails to insert (XM-6), decimal fields
  through JOIN are still scaled/mangled (XM-7b), and calendar creation
  through the public API is still impossible (XM-8). `ts` and `bal`
  remain fully working, unaffected either way.
- Both report documents (the full consolidated log and the pending-
  only filtered view) updated to reflect this and re-delivered.

## [0.7.12] — 2026-08-11

- **CRM example: JOIN re-enabled** now that XM-3a is genuinely fixed —
  a new saved query, "Deals with their company's industry", a real
  `INNER JOIN` (`deals AS a INNER JOIN companies AS b ON a.company.id
  = b.id` — `.id`, not bare `company`, since a REF field is a
  structured object, not a bare foreign key). Verified end-to-end
  against a real, freshly-seeded instance, not just that it saves.
- **Investigated whether the real seed script (with `companies.address`
  intact) could be made to work again against v0.30.5, given XM-6 —
  no viable workaround found.** Schema registration has no opt-out
  from adaptation; the only way to avoid XM-6 would be dropping schema
  validation for `companies` entirely (losing `additionalProperties:
  false` and the `industry` enum constraint), which is a real
  regression, not a genuine fix. The real seed script stays correctly
  blocked on XM-6 upstream — verified this session's work using a
  temporary, address-stripped copy instead, as before.
- **Two new JOIN limitations found while building the new query, filed
  as XM-7**: aggregate functions (`COUNT`, `AVG`, etc.) still aren't
  supported in a JOIN's own `SELECT` list — confirmed directly, a
  different gap from XM-3a's `tenant_id` bug, never previously
  reported. And decimal fields come back scaled and mangled through
  JOIN specifically — `deals.amount` of `"333000.65"` (correct via
  direct fetch and non-JOIN OQL) reads as `33300065` through a JOIN
  (a consistent ×100 scale, decimal point stripped). `ORDER BY` still
  sorts correctly since the scale is constant, so the new saved query
  was kept with the issue documented in a code comment, rather than
  either shipping a silently-wrong number or dropping a genuinely
  useful query over a display bug.
- Checked xoluman's own register for any other work unblocked by the
  new xolu version — only T-04 (the keyring backend) remains open,
  still blocked on a real OS keyring service this sandbox can't
  provide.

## [0.7.11] — 2026-08-11

- **Integrated xolu v0.30.5** (up from v0.30.0). Full build, vet, and
  test suite green, no code changes needed on xoluman's side.
  `go.mod` updated to match (relative `../xolu` replace directive
  unchanged).
- **Re-verified every open item in the consolidated xolu report log
  against v0.30.5, independently, not on the changelog's word alone.**
  Full results in the updated report (delivered separately):
  - **XM-3a (JOIN's missing `tenant_id`) and XM-4 (DXP partial-update
    staleness) — both genuinely resolved**, confirmed by direct
    reproduction of the original failing queries against a real,
    freshly-seeded instance.
  - **XM-3b (`LIMIT`) — confirmed resolved as "won't implement,"
    `TOP N` is the real answer** and now genuinely works with `JOIN`
    too, since 3a is fixed.
  - **XM-5 (`OFFSET`/`FETCH NEXT` doesn't paginate) — still broken**,
    re-confirmed with fresh evidence, despite xolu's own v0.30.5
    changelog describing it as "verified end-to-end against a real
    server."
  - **XM-2 (`ts`/`cal`/`bal` client methods) — still entirely
    unaddressed**, checked directly.
  - **New: XM-6** — nested (non-REF) object fields fail to insert
    into an adapted table, a real, apparently unintended side effect
    of the XM-4 fix itself (schema adaptation now correctly reaching
    named tenants for the first time, exposing a latent gap in the
    adapted-insert path that blob storage's plain-JSON handling had
    always silently covered for). Currently blocks the real CRM
    example's own seed script from completing against v0.30.5 as-is —
    verified XM-3/XM-4 using a temporary, address-field-stripped copy
    of the seed script instead of changing the real one.
- The CRM example's own saved queries have not yet been updated to
  use `JOIN`/`TOP` now that XM-3a is fixed, and XM-6 still blocks a
  real seed run with the `address` field intact — both left for a
  follow-up pass.

## [0.7.10] — 2026-08-07

- **Integrated xolu v0.30.0** (up from v0.27.1). Full build, vet, and
  test suite green against the new version with zero code changes
  needed — no breaking changes found. Re-ran the full CRM example
  end-to-end (schemas, entities, both FSM defs, both DXP transactions,
  bal accounts, blob attachments, all 17 saved queries) against a
  live v0.30.0 instance — everything that worked before still works.
  - Checked directly, not assumed: v0.30.0's own changelog confirms
    `examples/crm` was independently removed from xolu itself
    ("the xoluman team has already improved on it and now maintains
    it as their own worked end-user example") — the same move made
    here at v0.7.7, arrived at independently on xolu's side.
  - None of the three specific findings reported earlier this session
    show any change in v0.30.0, re-confirmed directly against the new
    version: the `ts`/`cal`/`bal` client-method gaps from the letter
    (still zero `ts` methods, still no `CalListBookings`/
    `BalListAccounts`), both OQL JOIN bugs (missing `tenant_id` in
    generated JOIN SQL; `JOIN`+`LIMIT` parsed as two statements), and
    the DXP-partial-update graph staleness finding (a DXP-patched
    deal's untouched `amount` field still reads as `null` through
    Sulpher). Noted honestly rather than assumed fixed just because
    the version number moved.
  - **`go.mod` fixed**: the `replace github.com/ha1tch/xolu` directive
    had an absolute, sandbox-only path (`/home/claude/work/xolu`)
    baked into every checkpoint shipped this session — flagged
    earlier, not acted on until now. Switched to a relative path
    (`../xolu`), portable across any machine where xolu and xoluman
    are checked out as sibling directories, which is how this sandbox
    itself is laid out. Version requirement also updated to match
    (`v0.27.1` → `v0.30.0`).

## [0.7.9] — 2026-08-07

- **CRM example**: both queries OQL's own JOIN bugs (see v0.7.8) had
  blocked now work, via Sulpher instead — direct response to "can you
  obtain using Sulpher what you couldn't obtain via JOIN?" Both
  confirmed by actually running them, not assumed from the grammar:
  - "Average deal size by company industry" — aggregation over a
    graph traversal, real openCypher syntax (`MATCH (d)-[:company]->(c)
    RETURN c.industry, count(d), avg(d.amount)` — implicit GROUP BY, a
    non-aggregate return item alongside aggregate functions).
  - "Contacts with no logged activity" — the genuine anti-join OQL
    couldn't express at all. `OPTIONAL MATCH` + `WITH ... WHERE ... IS
    NULL` is real openCypher's own equivalent, confirmed directly — 8
    real contacts with zero activities.
  - **A real, reproducible data-correctness finding surfaced while
    confirming the first query**: deals whose `stage` was set via the
    `close_deal_won`/`mark_deal_lost` DXP transactions show their
    `amount` field as `null` through Sulpher graph queries specifically
    — a field that DXP transaction never touches — even though a
    direct REST fetch of the same deal confirms `amount` is present
    and correct. Reproducible from the seed script itself. Documented
    in a full writeup (delivered separately) and in a code comment
    next to the affected query; not fixed, no xolu source touched.
  - "Average deal size by stage" (OQL, single-table, no JOIN) restored
    alongside the new industry-based Sulpher query — different,
    complementary dimensions, not a replacement for each other.

## [0.7.8] — 2026-08-07

- **Graph node/edge editing** — the graph viewer's inspect panel is
  now a real editable form, not read-only. Click a node, edit its
  scalar fields (system fields and REF-typed fields stay read-only —
  retargeting a relationship is a structural graph change, handled
  separately), Save writes through `POST /connections/{name}/query/graph/node`
  (`internal/ui/query.go`), a plain `Client.Patch` on whichever entity
  the node actually is. Click an edge, retarget it — `POST /connections/{name}/query/graph/edge`
  patches the *source* entity's own REF field (xolu's graph model
  derives edges from REF fields, there's no separate relationship
  object to update), then re-runs the current query, since retargeting
  genuinely changes the graph's shape. Verified end-to-end with real
  mouse clicks and direct API fetches confirming the changes actually
  persisted — a deal's name changed via the form, and a `primary_contact`
  edge retargeted to a completely different contact at a different
  company. 9 new Go tests.
- **DXP presets** — saved, form-driven DXP invocations, distinct from
  the existing raw-definition picker. A new resolver registry
  (`internal/ui/dxppreset.go`) turns a couple of simple form fields
  into a full binding set by looking up related data server-side —
  `close_deal_won`'s preset asks only for a deal ID and an optional
  note; `mark_deal_lost`'s asks for a deal ID and a reason. Both
  derive contact/owner/amount from the deal itself, the same way the
  CRM seed script's own direct invocations already did. New endpoint:
  `POST /connections/{name}/dxp/preset-run`. `dxp-editor.js` gained a
  "Saved presets" section above the existing raw-definition picker.
  `xoluman_saved_query` extended with a `dxp` mode (`dxp_def_name`/
  `resolver`/`form_fields`) — `ListSavedQueries`/`CreateSavedQuery`
  were hardcoded to reject anything but oql/sulpher/rest; fixed. 10
  new Go tests.
- **CRM example**: `mark_deal_lost` DXP def added (participants:
  update the deal, log why, create a follow-up debrief task — a
  genuine third participant, not padding; xolu's DXP transactions
  currently only implement the "3ps" pattern, confirmed directly by
  trying 2ps first and reading the real rejection, XOLU-DXP006), both
  presets registered as saved queries, plus 3 more OQL and 2 more
  Sulpher queries.
  - **Real, corrected understanding of two things initially gotten
    wrong**, both verified by actually running queries rather than
    assumed: OQL genuinely does support JOIN (missed real
    infrastructure — `sqlgen_join.go` plus five dedicated test files —
    on a first, too-narrow look), but two real bugs block using it
    against a live tenant today — a `tenant_id` column missing from
    generated JOIN SQL, and `JOIN` combined with `LIMIT` parsed as two
    separate statements. Every JOIN-dependent saved query was rewritten
    as single-table `GROUP BY`, and one genuine anti-join case
    ("contacts with no logged activity") was dropped entirely, since
    no rewrite could express it without working JOIN. Separately,
    Sulpher rejects multiple `MATCH` clauses before a `WITH`
    (`XOLU-GR004`) — real openCypher's own comma-separated-pattern
    form works correctly and was used instead. Full writeup of the two
    JOIN bugs delivered as its own report, not filed as xolu tracking
    items (not our call to make).

## [0.7.7] — 2026-08-07

- **`examples/crm` relocated from xolu into this repo**, at
  `examples/crm/` — the launcher, seed script, and README, unchanged
  in substance. Horacio's own review: the seed script's saved-queries
  step is genuinely xoluman-specific bookkeeping
  (`xoluman_saved_query`, an entity type only xoluman itself reads
  from; one query's own name references xoluman's graph viewer
  directly), grafted onto what was meant to be a generic, client-
  agnostic xolu example — and maintaining the same example across two
  repos risked two slowly-diverging copies of the same thing. Filed
  and closed as xolu's own T-164 (v0.27.3), which now no longer
  carries this directory at all.
  - The launcher's build step previously assumed it ran from inside
    xolu's own source tree (true only by construction, while it lived
    in xolu's own `examples/`). Reworked to take an explicit
    `--xolu-source /path/to/xolu` (or `XOLU_CRM_XOLU_SOURCE`) when
    building from source, with a clear error — not a cryptic `go
    build` failure — when neither that nor `--skip-build` (pointing
    at an already-built binary via `XOLU_CRM_BIN_PATH`) is given.
  - Verified all three paths end-to-end from the new location: build
    from an explicit `--xolu-source`, `--skip-build` against an
    already-built binary, and the missing-both error path.
  - `.gitignore` updated for the two new runtime-artifact locations
    this introduces (`/bin/`, `/examples/crm/xolu-crm-data/`).

## [0.7.6] — 2026-08-06

- **T-22 closed** (see `docs/RESOLVED.md`): backup/export UI. An
  `Export` button on the connection row (matching Test/Delete's
  placement) triggers `GET /connections/{name}/export`, which streams
  a full tenant `.zip` straight through as the HTTP response via
  `Client.Export` — correct `Content-Type`/`Content-Disposition`
  headers, no server-side temp file. Deliberately no timeout shorter
  than xolu's own polling — `r.Context()` is already tied to the
  browser's connection lifecycle, the right cancellation signal for a
  slow-but-healthy export. 3 new tests.
  - Real end-to-end testing against the CRM example — not just the
    unit tests — surfaced a genuine bug in xolu itself, not xoluman: a
    bal-enabled tenant that had never run a rollup failed the export
    outright, because bal's own setup eagerly creates its rollup
    store's directory on startup before any rollup ever runs, and
    xolu's `os.Stat`-based "never used" skip check saw a real
    directory and didn't skip it — `pebble.Open(ReadOnly:true)` then
    can't initialize the empty directory as a database. Filed and
    fixed upstream as xolu's own T-163 (v0.27.2); see that project's
    changelog for the full account. Re-verified afterward: a real
    tenant export against the live CRM data now downloads a valid,
    non-empty zip (29 files, including the bal/dxp/fsm data from this
    session's own CRM extension) through xoluman.
- Only one item remains open in the register: T-04 (the keyring
  backend's live round-trip verification), which needs a real OS
  keyring service to run against and can't be completed in this
  sandbox.

## [0.7.5] — 2026-08-05

- **Graph viewer** — a new page (`/connections/{name}/graph`, "Graph"
  in the connection dropdown) for visualizing arbitrary Sulpher query
  results as a real diagram: type a query, run it, see the nodes and
  edges, click either to inspect its data. Built as a genuine
  extension of the FSM editor's canvas, not a second widget — direct
  response to "a tailored version of the fsm-editor widget could do
  that job without duplication, if we add configuration parameters."
  - `internal/ui/query.go`: a new `POST /connections/{name}/query/graph`
    endpoint runs the query via `Client.GraphQuery` and reshapes the
    raw result rows into `{nodes, edges}`. The classification rule
    (a node has `_id`+`type`; an edge has `from`/`rel`/`to` and no
    `_id`) was confirmed by tracing Sulpher's own executor and then
    running a real query against the CRM dataset to see the actual
    JSON — not assumed from the source alone. Deduplicates both across
    rows (normal for a multi-hop query, where the same node or edge
    legitimately appears more than once). 4 new tests.
  - `web/static/js/fsm-editor.js`: a `mode="graph"` branch throughout
    — a query input and Run button replace the machine name/state/
    transition fields, a read-only inspect panel replaces the
    validation panel, and the FSM-editing hints (add state, add
    transition) are replaced with "click a node or edge to inspect
    it." Node labels pick a sensible display field (name/title/first+
    last name) from the entity's own data rather than showing a raw
    `"companies:6"` on the canvas. Selection tracking hooks the same
    wrapped `draw()` used for fsm-mode's validation/layout-save
    triggers, since the engine has no native selection-changed event
    either way.
  - Verified end-to-end with real mouse clicks, not simulated
    selection: computed actual screen coordinates from the engine's
    own `viewport` transform (`{x, y, k}`, confirmed against
    `crossBrowserRelativeMousePos`'s own formula), clicked a real node
    and a real edge midpoint, confirmed the inspect panel showed the
    genuine entity data and the genuine `from`/`rel`/`to` respectively
    — against real multi-hop CRM data (deals → contacts), in both
    light and dark mode.

## [0.7.4] — 2026-08-05

- **FSM editor canvas rebuilt on Seam's actual widget, replacing the
  from-scratch SVG canvas shipped in v0.7.3.** Direct question from
  Horacio ("did you create an entirely new SVG widget instead of
  using the one that came with Seam?") led to actually reading
  `seam-fsm-editor.js` for the first time this session, rather than
  trusting a prior session's summary that had gone stale. That file
  turned out to be Seam's own substantially extended fork of Evan
  Wallace's Finite State Machine Designer (MIT), not the bare
  upstream — layered interaction animations (a pulsing in-progress
  link, an eased snap-back when a drag fails to connect, hover/
  selection halos, a pulsing group-selection halo for rubber-band
  multi-select, momentum-based zoom, smooth label repositioning),
  per-link `guard`/`action` properties with their own dialog, per-link
  custom colours, and JSON/SVG/LaTeX export — none of which the v0.7.3
  rebuild attempted to reproduce.
  - `web/static/js/fsm-canvas-engine.js`: Seam's engine, copied in
    directly with full MIT attribution, not rebuilt. One addition on
    top of the fork: `linkProperties` gained a third field, `output`,
    alongside the fork's existing `guard`/`action` — xolu's transition
    model has a distinct output value neither Wallace's original nor
    Seam's fork had a field for. Every place `guard`/`action` are
    read, written, saved, or restored was extended to carry `output`
    the same way.
  - `web/static/js/fsm-editor.js`: replaced entirely — a new,
    deliberately thin Lit shell modeled closely on Seam's own
    integration pattern (the same canvas-context `Proxy` theming
    trick, the same documented Engine API), owning only what's
    specific to xoluman: conversion between the engine's own backup
    format and xolu's real `MachineSpec`, local fsm-toolkit
    validation, and layout persistence as `xoluman_fsm_layout` — split
    out from the engine's combined backup object on save and merged
    back in on load, keeping the "layout separate from the abstract
    machine" principle even though the engine's own native format
    keeps them together.
  - Disclosed data-model gap: xolu's `Set` (a map of potentially
    several variable assignments) is represented via the engine's
    single free-text `action` field as a semicolon-separated
    `key=value` list, parsed and re-serialized each round trip — a
    disclosed convention, not a structured multi-assignment editor.
  - **A real bug found by testing, not by reading the code**: the new
    converter wasn't auto-populating `output_alphabet` from
    transitions' output values, which xolu requires — every
    transition carrying an output was unconditionally rejected.
    Fixed, then verified for real: injected a new terminal state with
    a transition carrying both an `output` and a `set` clause, saved
    it, and confirmed via direct API fetch that xolu accepted and
    persisted every field correctly (including the correct
    `action`→`set` conversion), with the layout intact alongside it.
  - Light/dark theme support re-created (not carried over — only the
    engine file, not Seam's own shell, was reused) using the same
    proxy/palette/global-variable-bridge approach as Seam's own shell.
    Verified via computed style in both themes and a live,
    no-reload theme toggle, not just code presence.
  - Zero page errors across every test scenario; full CRUD (create,
    load, edit, save) re-verified end-to-end against a real xolu
    instance with the new engine in place.

## [0.7.3] — 2026-08-05

- **T-14 closed** (see `docs/RESOLVED.md`): the FSM def module — a
  full visual viewer and editor, not the read-only version originally
  scoped, per direct request.
  - `internal/ui/fsmdef.go`: full CRUD (List, New/Edit shell, GetData,
    Create, Update, Delete) against `client.CreateMachineDef`/
    `ReplaceMachineDef`/`DeleteMachineDef`, using the real
    `MachineSpec`/`TransitionDef` wire shape directly — every field
    (guard, output, variable `Set`), not a lossy collapse into one
    label the way Seam's own canvas engine was found to do in an
    earlier assessment of reusing it.
  - **Validate before submission**, via `github.com/ha1tch/fsm-toolkit`:
    converts a `MachineSpec` into fsm-toolkit's own `*fsm.FSM` (states,
    initial state, and transitions map cleanly; guards, variable `Set`
    clauses, GC policy, and input queries don't, and are surfaced as
    explicit skipped-check notes rather than silently ignored) and
    runs `Validate()`/`Analyse()` — debounced, live, zero network
    calls to xolu. Complementary to xolu's own `ValidateMachineDef`,
    not a replacement for it.
  - **Visual-diagram persistence**, following fsm-toolkit's own
    `fsmedit` separation (`pkg/fsmfile.Layout` — position data kept
    structurally apart from the abstract machine) — a new
    `xoluman_fsm_layout` bookkeeping entity, same established pattern
    as `xoluman_field_meta`, not fsmedit's own zip+hex+TOML file
    format (built for local files on a TUI tool, not a web app
    talking to a remote xolu instance).
  - `web/static/js/fsm-editor.js`: a real SVG canvas — draggable
    states, transition arrows (including self-loops), a live
    validation panel.
  - **Three real bugs found and fixed only by actually running it
    (Playwright), none visible from reading the code**: a panic on
    the list page from passing an uncalled `mi.H` render function as
    a child instead of invoking it; transition `from` fields
    double-JSON-encoded; and — the significant one — the entire SVG
    canvas rendering in the wrong DOM namespace, because nested
    per-state/per-transition sub-templates used Lit's `html` tag
    instead of its dedicated `svg` tag, silently turning `<circle>`/
    `<path>`/`<text>` into unrecognized HTML elements with zero
    bounding boxes and no working drag. Confirmed via `getBBox()`
    (didn't exist) and `namespaceURI` (`xhtml`, not `svg`) before
    fixing it, then verified after with a real drag-and-persist round
    trip — dragged a state, re-fetched the layout from the server,
    confirmed the actual new coordinates.
  - Also fixed along the way: the new JSON API endpoints
    (Create/Update/Delete/GetData/ValidateLocal/SaveLayout) were
    returning `writeUpstreamError`'s full HTML error page on
    failure — correct for page handlers, broken for a JS component
    reading `fetch()` responses as JSON — given a dedicated
    `writeUpstreamErrorJSON` path instead of touching the shared,
    page-handler-wide function.
  - Known, disclosed limitation, not a bug: two transitions between
    the same state pair draw overlapping labels at the same midpoint
    — a curve-apart fix, not attempted this pass.
  - 10 new Go tests for the converter/validation logic. Full CRUD —
    create, read, update, delete, local validation, layout save —
    verified end-to-end against a real xolu instance, not just
    unit-tested.

## [0.7.2] — 2026-08-04

- **T-15 closed** (see `docs/RESOLVED.md`) — grid select-field support
  was the last open piece. `buildGridColumns` now consumes
  `xoluman_field_meta` the same way the entity form's own select
  fields already do: a configured field gets Tabulator's `list`
  editor with the real value→label mapping, instead of a bare text
  input. Verified end-to-end, not just unit-tested: a real
  `field_meta` entry, a real edit through the dropdown, a real save,
  and a direct API check confirming the value actually changed
  (`small` → `enterprise`) — checked in both light and dark mode. 5
  new tests.
- **T-21 closed** (see previous entry, v0.7.1) — register housekeeping
  only, already resolved in practice.

## [0.7.1] — 2026-08-04

- **T-21 closed** (see `docs/RESOLVED.md`) — was already resolved in
  practice as of the v0.27.0 integration pass (`Client.TestConnection()`
  wired into both `Test`/`TestUnsaved` handlers), just never formally
  closed in the register. No code change this release; register/docs
  housekeeping only.

## [0.7.0] — 2026-08-04

Minor bump, not a patch — a genuine architectural change (the header
redesign) and a new feature (lightning-icon navigation) alongside a
long list of real bugs found and fixed, several only catchable by
actually running a browser rather than reading source. Playwright
(browser automation) turned out to be available in this sandbox and
was used throughout this pass — screenshots, click-throughs, computed-
style inspection — which is how most of what follows was actually
found, not assumed from code.

- **The real fix for "saving is broken" (T-24's remaining half)**:
  a ref field whose schema doesn't declare a target had no way to be
  written to more than once, since the companion entity-type input
  could never be pre-filled from a plain read (xolu's embedded
  read-shape doesn't self-identify its own entity type). Fixed
  properly: extended the existing `xoluman_field_meta` bookkeeping
  pattern (`fieldmeta.RememberRefTarget`/`LookupRememberedTarget`) so
  a target is remembered the first time it's specified and auto-fills
  for every future edit of that field, on any row. Verified
  end-to-end across two different rows — type it once, never again.
  A related, smaller gap fixed in the same pass: the list preview and
  the form's own ref-link building both still only checked
  schema-declared targets, not remembered ones, so a remembered field
  wouldn't show a ref link (or the new lightning icon) at all — both
  now use the same expanded, memory-aware lookup.

- **Modal title consolidation** — a fragment's own `<h1>` (e.g. "Edit
  companies #1") is now promoted to the modal's title bar and removed
  from the body, replacing the less specific static title the trigger
  button set at click time. One centralized fix (`modal.js`'s own
  `htmx:afterSwap` listener) rather than coordinating title text
  between every Go handler and its trigger button.

  **Found and fixed two real, related bugs building this feature, both
  only catchable by actually clicking through it:**
  - The first attempt listened on `document.body`, which doesn't
    exist yet when `modal.js` runs (a synchronous `<script src>` in
    `<head>`, executing before `<body>` is parsed) — an uncaught
    exception that silently broke `XModal`'s entire IIFE, meaning
    *every* modal (New/Edit/Delete) would have stopped working.
    Fixed: listen on `document` instead.
  - Building the lightning-icon feature (below) surfaced a second,
    related bug in `XModal.open()` itself: opening a new modal from
    *within* an already-open one (the lightning icon's exact use
    case) destroyed the current modal's DOM — including the very
    button whose click triggered the open — before htmx's own click
    handler on that same element could fire its request, so the
    request silently never happened. A second attempt (leaving the
    modal alone but resetting `#modal-body`'s own `textContent`) made
    the identical mistake one level down, since the button lives
    inside `#modal-body` too. Fixed properly: `open()` no longer
    touches `#modal-body`'s content synchronously at all when reusing
    an existing modal — the old content stays visible until htmx's
    own swap replaces it once the response actually arrives.

- **Header redesign** — the persistent left sidebar (shipped last
  pass) is gone, reported directly as looking bad where it was.
  Replaced with: the connection name genuinely centered in the nav bar
  (confirmed pixel-exact via Playwright, not just visually plausible),
  a native `<details>`/`<summary>` dropdown menu (Entities/Blobs/
  Query/DXP, All connections, a connection switcher) — zero additional
  JS, browser-native open/close, deliberately not a second custom JS
  component to get subtly wrong the way `modal.js` just was — and the
  theme toggle repositioned to the top right. The old registry-driven
  top-level nav-links mechanism (unused in practice — no module had
  ever set a Label) was retired along with its tests.

- **Theme toggle visibility — found and fixed, likely the real
  explanation for "we lost the theme switcher, it's always dark."**
  The toggle mechanism itself tested correctly in every scenario
  across multiple sessions. The actual bug only showed up in an actual
  screenshot: `mi.DarkModeSVGIcons()`'s icon markup (`class="w-5
  h-5"`) is generated inside minty's own library code, outside
  Tailwind's content-scanning paths (`./internal/**/*.go`), so no CSS
  was ever generated for those classes — the icon rendered at zero
  size. Functionally clickable the whole time, but invisible; if you
  can't find the button, you can't toggle it, and a dark OS preference
  would then look permanently stuck. Fixed with a Tailwind `safelist`
  entry, documented for why, and confirmed visible in a real
  screenshot afterward.

- **Grid dark mode — root-caused properly, not re-patched.** A
  screenshot revealed the real problem: Tabulator's own selectors are
  frequently 3-class compounds (e.g. `.tabulator .tabulator-header
  .tabulator-col`) that beat a 2-class override (`.dark
  .tabulator-col`) on CSS specificity regardless of source order — the
  first version of the override file only had `!important` on two
  rules, added ad hoc, and was silently losing to Tabulator's own
  light-theme defaults almost everywhere else. Rewrote the whole file
  with `!important` throughout — the correct, deliberate pattern for a
  dedicated third-party override file, not a shortcut — plus several
  previously-missing selectors (`.tabulator-table`, the pagination
  footer's page buttons, `.tabulator-col-title`). Also found and fixed
  a **pre-existing light-mode bug** in the same pass, same root cause:
  an empty gray area below the last row in light mode too (Tabulator's
  base `background:#888` showing through). Both themes fully verified
  via screenshot, not just computed-style spot checks.

- **Lightning-icon ref navigation** — a small SVG bolt icon next to
  ref links (both the entity list preview and inside edit forms) that
  opens the linked entity in a modal instead of a full-page
  navigation. Verified end-to-end: clicked it for real, confirmed the
  request fires, the modal title updates to the linked entity's own
  title, and the body genuinely shows the linked entity's own fields
  (not the original one again). Breadcrumbs and side-by-side viewing,
  both mentioned as possible directions, are deliberately out of scope
  for this pass.

## [0.6.23] — 2026-08-04

- **xolu v0.27.0 integrated — the xolu team's response to
  `docs/xolu-requests.md`, all claims verified directly against
  source before trusting them, not taken on the letter's word alone.**
  T-23 and T-24 closed (see `docs/RESOLVED.md`).
  - **T-23 (tenant-scoped schema-endpoint prefixing): already fixed,
    in v0.26.2, before the letter even arrived.** Confirmed directly —
    `buildURLRoot` now exists, `GetEntitySchema`/`DefineEntitySchema`/
    the schema-list call all correctly skip the tenant prefix.
    `xoluext.BuildSchemaClient()` and `schemaClientFor` removed
    entirely; all 6 call sites reverted to the regular client. 2
    obsolete tests replaced with one confirming the *regular* client
    now handles this correctly on its own.
  - **T-24 (the real "saving is broken" cause): xolu's diagnosis
    corrected ours.** Our own working theory (undeclared ref target)
    reproduced the symptom but was wrong about the cause — a plain
    schema with no ref fields at all reproduced the identical failure.
    Real cause: `PUT`/`PATCH`/`save` validated a document that already
    contained `id` (and `_version` for `PATCH`) — system fields no
    schema declares — against `additionalProperties:false`. Fixed
    server-side (`stripSystemFieldsForValidation`); verified
    independently with a direct, correctly-shaped `PUT` against the
    exact `examples/crm` repro.
  - **A second, entirely separate bug found once T-24's fix was
    verified — xoluman's own, nothing to do with xolu.** Updating
    `companies` through xoluman's own web form *still* failed after
    the id/_version fix, because the form submitted a bare number for
    any ref field whose schema doesn't declare a target (confirmed:
    exactly what `examples/crm`'s own seed script does for every ref
    field except `users`'). Affected both create and update
    identically. **Fixed properly**: `formengine.RenderOptions` gained
    `RefTargets`; `refInput` now renders a companion
    `f.Name+"__ref_entity"` input specifically when a ref field's
    target is unknown, letting the person supply it directly;
    `ParseFormValues` uses it to build the real
    `{type,entity,id}` shape. Degrades honestly to the old
    bare-number behavior (and xolu's own clear rejection) when left
    blank — no silent guessing. Pre-fills correctly when redisplaying
    after a validation error, from the write-shape's own `entity`
    field. 9 new tests across `formengine`/`formengine_test`/
    `parse_test`. **Verified end-to-end through xoluman's actual web
    form** against the real CRM demo — both create and update now
    genuinely succeed, confirmed persisted in the real data, not just
    each half in isolation.
  - **A severe bug found while verifying #5 below, fixed on both
    sides**: `apikey` auth mode sent `Authorization: Bearer <key>`
    since the option existed; the server's own validator only ever
    accepted `X-API-Key`/`Authorization: ApiKey <key>`. Every
    `apikey`-configured client was silently unauthenticated on every
    request. Confirmed empirically, not just read the fix — against a
    real, credential-enforcing v0.27.0 server, the old header
    genuinely 401s, the new one genuinely 200s. Found xoluman's own
    test for this had the identical blind spot xolu's team found in
    theirs: a mock recording what header was sent, which would have
    passed against the broken behavior too. Fixed (renamed from
    `..._SendsBearerToken` to `..._SendsAPIKeyHeader`, corrected
    expectation).
  - **#5 (`Health()` doesn't verify a credential): delivered, new
    method.** `Health()` is correctly, deliberately unauthenticated by
    design (matches `/ready`/`/version`/`/metrics`) — the fix was
    calling the right method, not changing that one. Both connection-
    test handlers switched to the new `Client.TestConnection()`. New
    test proves the actual point: a server that's reachable but
    rejects the credential now correctly reports failure, which
    `Health()` structurally could never detect (mock server built to
    genuinely distinguish this — `/health` unconditionally succeeds,
    matching real xolu, while `/api/v1/schemas` actually checks the
    credential).
  - `docs/xolu-requests.md` updated throughout to reflect all of the
    above accurately — every one of the 9 original items now struck
    through as delivered.
- **xolu v0.27.1 synced — reviewed directly, confirmed a no-op for
  xoluman.** The only change in that release is client-side validation
  added to `CreateMachineDef`/`ReplaceMachineDef` (Seam AMS's own
  request, T-162) — confirmed via a full recursive diff of
  `pkg/client` that nothing else changed, and confirmed xoluman calls
  neither method (T-14, the FSM def module, isn't built yet). Clean
  version bump, full test suite green, real e2e sanity check against
  a running v0.27.1 instance.

## [0.6.22] — 2026-08-04

- **T-25 closed: DXP transaction support built** (see
  `docs/RESOLVED.md` for what T-25 originally covered). A new,
  standalone view — `/connections/{name}/dxp`, reached via the sidebar
  — rather than a fourth query-editor mode, per the proposal's own
  reasoning (`docs/proposals/dxp-transactions.md`): a DXP transaction
  has no query language, only a chosen definition and the parameter
  values ("bindings") its participants reference.
  - **Def picker**: lists every registered definition
    (`DxpDefList`), alphabetically.
  - **Binding extraction, server-side** (`internal/ui/dxp.go`): walks
    a selected def's full participant set (`DxpDefGet`), collecting
    every distinct name referenced via `{"$ref": "..."}` — including
    nested inside an array or another object, not just a top-level
    param value. One implementation, not duplicated in JS. 7 new
    tests covering top-level, nested-in-object, nested-in-array,
    cross-participant deduplication, sort stability, and two
    deliberate non-matches (a literal value with no `$ref`; a
    multi-key object that happens to contain a `"$ref"` key but isn't
    the documented single-key ref shape).
  - **Dynamically-generated binding form** (`web/static/js/dxp-editor.js`):
    one input per extracted binding name, accepting a bare number,
    `true`/`false`, or a string — parsed as JSON with a plain-string
    fallback, so typing intuitively produces the right type in the
    `Bindings` map without needing to pre-know each binding's type.
  - **Execution and result display**: `DxpTxnCreate`, with a
    non-committed outcome (`released`/`expired`) rendered as a
    distinct, informative state — confirmed directly against the
    client's own doc comment that this is a normal response, not an
    error, and implemented that way (`Run` always returns 200 with
    the real status for genuine outcomes; only a transport/validation
    failure before a transaction was even attempted uses the
    upstream-error path). 6 new handler tests, including one
    specifically asserting the released-is-not-an-HTTP-error behavior.
  - Verified end-to-end against a real xolu instance with `XOLU_API_V2_ENABLED=true`:
    registered a genuine DXP def, confirmed binding extraction against
    the real (not synthetic) server response, ran a transaction with
    real binding values, and confirmed the entity it created actually
    exists in xolu afterward — the full flow, not just each piece in
    isolation.
  - "Saved DXP invocations" (a saved def+bindings pair) not built —
    the proposal named this as its own deliberately separate concept
    from the query-editor's saved-query feature; not part of this pass.

## [0.6.21] — 2026-08-04

- **Saved and recent queries, for all three query editor modes
  (OQL, Sulpher, REST) separately.**
  - **Saved**: explicit, named, server-backed — a new schema-less
    `xoluman_saved_query` entity type, same established pattern as
    `xoluman_field_meta` and `xoluman_blob_folder`: persisted in the
    connected xolu instance itself, so a saved query is visible to
    anyone else using xoluman against the same instance, not locked
    to one local xoluman installation. New JSON API
    (`internal/ui/query.go`): list (mode-filtered, newest first),
    create, delete. 8 new Go tests.
  - **Recent**: local, ephemeral, per browser — the last 15 queries
    actually run per mode, kept in `localStorage`, scoped per
    connection (keyed off the run URL, which already encodes the
    connection name) so switching connections doesn't mix histories.
    Recorded on every run regardless of outcome — a failed query is
    one someone might want to pull back up and fix, not just a
    successful one worth remembering.
  - Both surfaced as two dropdowns above the editor in
    `web/static/js/query-editor.js`, plus a "Save…" button that opens
    an inline name field (not a native `prompt()` dialog) and a small
    delete control for the currently-selected saved entry.
  - Verified end-to-end against real xolu: mode-filtering genuinely
    isolates OQL/Sulpher/REST saved queries from each other, the real
    xolu entity is created with the correct fields per mode, and
    deletion is real (confirmed gone from a subsequent list, not just
    a 204 with no follow-through).
- **`docs/proposals/dxp-transactions.md` — DXP transaction support
  considered and written up, not built.** `DxpDef`/`DxpTxn` confirmed
  directly against the real client — a def picker plus a dynamically-
  generated binding-fill form, not a query language, so proposed as
  its own view rather than a fourth query-editor mode. Filed as T-25,
  explicitly deferred rather than silently dropped.

## [0.6.20] — 2026-08-04

- **Connections sidebar shipped — Phase 1 of the DBeaver-layout
  proposal, in a scoped first form.** Direct response to a real
  report: returning to the connections list to switch context, or to
  reach Blobs/Query after already being in Entities, made no sense
  once already working inside a connection. A persistent sidebar now
  appears on every `/connections/{name}/...` page: current connection
  name, Entities/Blobs/Query links with active-state highlighting
  (correctly active on subpaths too — browsing into a blob folder
  still highlights Blobs), and a "switch connection" list of every
  other saved connection.
  - Parsed entirely from the URL path already being rendered
    (`connectionNameFromPath`) — no existing page handler needed a
    signature change to thread a connection name through.
  - The connection list for the switcher comes from a new
    package-level `sidebarStore`, mirroring the exact pattern already
    established for the module registry (`registry` in the same
    file) — same trade-off, already accepted elsewhere in this
    codebase, not a new kind of risk.
  - Deliberately the simpler of two designs from the proposal: regular
    server-rendered navigation, not `hx-boost` — the sidebar re-renders
    on each page load like everything else today. The `hx-boost`
    refinement (sidebar DOM persists across navigation) is still
    available as a real follow-up; not pursued here since this already
    delivers the actual value asked for, with less risk around the
    existing htmx-based modal wiring.
  - 5 new tests, including one with the shuffled test order and race
    detector specifically to check the global `sidebarStore` for
    cross-test leaks — none found.
  - Verified end-to-end against a real running instance: correct
    active-highlighting on Entities/Blobs/Query and on a Blobs
    subpath, the switcher correctly excluding the current connection,
    and correct handling of a connection name with a space (decoded
    for display, properly re-escaped in hrefs).

## [0.6.19] — 2026-08-04

- **Table styling redesigned to match minty's own established
  conventions** — reported directly as looking "too default, no signs
  of styling attempts." xoluman depends on minty throughout but had
  never adopted its own Tailwind theme's table convention
  (`themes/tailwind/tailwind.go`'s `Table()`): a real elevated card
  (shadow, subtle ring, rounded corners) rather than a bare table
  sitting flush on the page background, a distinct header background
  with uppercase tracked text instead of plain text, and striped rows.
  `internal/ui/listing.go`'s `Table()` — used by every list page in the
  app — now applies all three. Striping is a single Tailwind arbitrary-
  variant selector on `<tbody>` (`[&>tr:nth-child(even)]:bg-gray-50`)
  rather than requiring every caller to stripe its own rows, so every
  existing call site picked it up automatically.
- **The grid view's Tabulator mount got the same treatment** — it had
  no wrapper styling at all (Tabulator's own vendored theme covers
  rows/cells/headers, nothing at the outer-frame level), now wrapped
  in a matching elevated card in both light and dark mode.
- 1 new test, full suite green, Tailwind CSS rebuilt and the new
  classes confirmed present in the compiled output before shipping.

## [0.6.18] — 2026-08-04

- **`docs/xolu-requests.md` rewritten as a single, living status
  letter** consolidating all four requests filed to the xolu team
  (the original 6-item batch, the FSM-def follow-up, and the two
  newly-discovered bugs — T-23, T-24) into one place: delivered items
  struck through with the version that shipped them, open items marked
  🔴 ACTIVE, nothing left implicit. Two items previously believed still
  open were re-checked directly against the current xolu source while
  writing this, not carried forward as stale assumptions: item #2's
  `EXPORT_API.md` doc bug is fixed (correctly reads `xolu.db`/
  `database_file` throughout now), and item #6's `FieldDef.Type` doc
  comment now explicitly states it's never set to `"ref"` — both
  closed out. Genuinely still open, re-verified the same way: `Health()`
  still applies no auth header (#5), the tenant-schema-prefix bug (#8,
  T-23) and the undeclared-target REF update failure (#9, T-24).
- Scanned `KNOWN_ISSUES.md` for anything found this session but never
  formally written up as a request — found nothing outstanding; every
  actionable xolu-side finding was already filed as its own document.
- **Finished last session's interrupted migration to minty's own
  `DarkMode` mechanism**, replacing the hand-rolled `theme.js` and
  custom toggle button — `PageWithHead` now uses
  `mi.DarkModeTailwind(mi.DarkModeSVGIcons())`, giving real SVG
  moon/sun icons and minty's own tested init/persistence/icon-update
  logic instead of a from-scratch reimplementation of the same thing.
  `theme.js` deleted. Two tests fixed in the process: one had a false
  positive (a blanket `"http://"` substring check flagged the SVG
  icons' own `xmlns="http://www.w3.org/2000/svg"` namespace URI as an
  external reference — narrowed to check actual `src=`/`href=`
  attribute values instead), the other rewritten to check minty's
  real generated markup rather than the removed custom implementation.

## [0.6.17] — 2026-08-04

- **T-23: found and fixed the actual root cause of "clicking any
  entity gives XOLU-ST004: Invalid ID"** — a bug that survived two
  prior sessions of investigation and a whole (real, but ultimately
  unrelated) URL-escaping fix, because every earlier test used
  `AuthType=none` with no tenant configured, and this only manifests
  for a tenant-scoped connection.
  - Root cause, confirmed precisely: `Client.buildURL` applies the
    tenant path prefix to every request once a tenant is set, with no
    per-endpoint awareness of which endpoints are actually
    tenant-scoped. `/schema/{entity}` (`GetEntitySchema`/
    `DefineEntitySchema`) is registered on xolu's server only at the
    global level — confirmed directly against the route table, never
    duplicated under the tenant router, unlike `/entities`,
    schema-suggestion, and both promote endpoints, which genuinely
    are. A tenant-scoped client's schema fetch for `companies`
    therefore requests `/api/v1/tenant/{tenant}/schema/companies` —
    xolu's router matches this against its entity-by-id pattern
    instead, landing `"companies"` in the numeric `{id}` slot, and
    `strconv.Atoi` fails exactly as `XOLU-ST004`.
  - Found by running xolu's own `examples/crm` demo — the realistic,
    tenant-scoped, six-entity-type dataset needed to actually surface
    this, after which it reproduced byte-for-byte via direct curl
    before a single line of xoluman code was touched.
  - Fixed: `internal/xoluext.BuildSchemaClient()` builds a second,
    tenant-less client per connection, used only for the two schema
    calls. All 6 real call sites migrated (`Show`,
    `resolveEntityFields` covering `NewForm`/`Create`, `EditForm`,
    `Update`, `GridView`, `ImportPreview`) — two of them (`GridView`,
    `ImportPreview`) had their now-unnecessary regular client fetch
    removed entirely rather than left dangling unused.
  - 7 new tests, full suite green.
  - Verified end-to-end against `examples/crm`: the exact
    previously-failing request now returns 200 with correct data, and
    a full sweep across all 6 entity types' list pages, edit pages,
    and every ref link (121 URLs total) found zero failures.
  - This is a real bug in xolu's client library, not something fixable
    from xoluman's side alone — filed as its own focused request
    (`docs/xolu-requests-tenant-schema.md`) with the exact reproduction
    and both possible fixes named plainly. xoluman's workaround is
    explicitly not meant to be maintained indefinitely once the client
    library closes the gap.

## [0.6.16] — 2026-08-04

- **Real bug fixed, systemically: connection names and entity type
  names were never URL-escaped anywhere they flowed into a path —
  across the whole UI layer** (`entities.go`, `grid.go`, `import.go`,
  `promote.go`, `blobs.go`, `connections.go`, `query.go`). Confirmed
  directly: a connection named "My Server" rendered a literal,
  unescaped space into every href built from it. A character with real
  meaning in a URL path — `/`, `#`, `?`, `%` — landing unescaped could
  plausibly shift what the server-side router captures for an adjacent
  path segment, a very likely explanation for the reported "clicking
  any entity gives XOLU-ST004: Invalid ID," though that couldn't be
  conclusively reproduced with a simple name before this fix. Verified
  end-to-end with a full click-through against real xolu — connections
  list → Entities link → entity type list → Edit link — using a
  connection deliberately named with a space, every hop correctly
  `%20`-escaped, landing on a working edit page.
  - Introduced `entitiesBasePath()` (escaped) as the shared helper for
    the most common pattern, replacing 16 raw-concatenation call
    sites.
  - The blob browser needed a more careful split: `joinPath` (kept
    unescaped, used only for hidden form field values — a browser's
    own form submission already percent-encodes those, so escaping
    here would double-encode) versus a new `joinPathForURL` (escaped,
    for actual hrefs, where a browser does *not* re-encode before
    navigating).
  - 5 new regression tests.
- **Theme consistency, several real gaps closed — direct
  consequence of trimming Seam AMS's dark-mode-aware widget pattern
  down to nothing when xoluman's own versions were first built,
  rather than carrying the pattern itself forward.** `modal.js`'s own
  history literally recorded the decision ("dark-mode detection... not
  yet needed") well before the theme toggle existed to need it, and it
  was never revisited once the toggle shipped.
  - `modal.js`: restored Seam's `document.documentElement.classList.
    contains('dark')`-at-build-time pattern (Seam's own SeamModal/
    SeamPopover convention), replacing the CSS `Canvas`/`CanvasText`
    system colors that were following the *OS* preference rather than
    xoluman's own explicit toggle state — the actual mechanism of the
    inconsistency.
  - Tabulator (grid view): confirmed it ships no CSS custom properties
    to hook into; added a hand-written `.dark`-scoped override
    stylesheet (`tabulator-dark-overrides.css`) rather than swapping in
    Tabulator's own separate "midnight" theme, which would have meant
    a second, inconsistent theming mechanism alongside the single
    class-toggle one xoluman already uses everywhere else.
  - CodeMirror (query editor): added `@codemirror/theme-one-dark`
    (MIT; its dependencies were already in the bundle), wired through
    a `Compartment` with a `MutationObserver` watching `<html>`'s
    class attribute — this editor can stay open across a toggle,
    unlike the modal, so it needed live reactivity, not a one-time
    check. Caught a real follow-on bug while wiring this in: the
    previous session's mode-switch fix builds a fresh `EditorState` on
    every tab change, and that fresh state also needed the theme
    extension included, or switching tabs would silently drop back to
    light.
  - Both Lit components' own chrome (tabs, buttons, result/error
    boxes) had the identical gap — static, light-only hex colors, no
    dark variants at all. Fixed to match xoluman's own established
    conventions (`green-600`/`dark:green-400`, `red-600`/
    `dark:red-400`, `indigo-600`/`dark:indigo-400`) rather than
    inventing new ad hoc colors per component.
  - The Go-rendered pages were checked too and found already correct —
    every color there goes through Tailwind's `dark:` utilities, not a
    gap of the same kind.
  - CodeMirror bundle re-vendored (hash and size updated in
    `VENDOR.md`); new `ownAuthoredCSSFiles` presence-check test added
    alongside the existing JS one.
- Verified end-to-end: theme.js, the Tabulator dark-overrides
  stylesheet, and modal.js's restored logic all confirmed served and
  correctly wired from their respective pages against a real running
  instance.

## [0.6.15] — 2026-08-04

- **README rewritten.** It had genuinely never been updated since the
  project's very first commit — still read "v0.0.1, scaffolding stage,
  no functionality shipped yet" at v0.6.14, referenced closed/
  superseded tracking IDs (T-01/T-02/T-03), and described features
  (CSV/XLSX/ODS export) that were never what actually got built. Now
  reflects the real, current feature set, with every linked file
  path verified to actually exist before committing to it.
- **`docs/proposals/dbeaver-layout.md` added** — a design proposal for
  a persistent sidebar tree + tabbed workspace, requested but not
  built (per the request, a proposal to review, not an implementation
  to ship). Splits the idea into a lower-cost sidebar-tree phase
  (achievable with `hx-boost`, no fundamental architecture change) and
  a genuinely expensive real-tabs phase (a client-side state rewrite),
  recommending only the first for now, with the trade-offs of the
  second stated honestly rather than glossed over.

## [0.6.14] — 2026-08-04

- **Real bug fixed: the Test button on the connections list did
  nothing for any connection whose name contained a CSS-selector-
  special character** (a period is completely ordinary in a real
  connection name — e.g. "prod.local"). htmx's `hx-target` is used as
  a literal `document.querySelector` CSS selector; the raw name went
  in unescaped, so `#status-prod.local` parsed as "id `status-prod`
  AND class `local`" and never matched the actual element — the swap
  silently failed with nothing to notice anywhere. Confirmed directly
  against the rendered HTML before fixing. Fixed by keying the status
  element's id to the row's index instead of the connection name,
  which is always CSS-safe regardless of what a connection is named.
  Regression test added.
- **Real bug fixed: the query editor's OQL/Sulpher/REST tabs shared
  one CodeMirror document.** Switching modes only reconfigured the
  language extension, never the document — reported precisely as "the
  OQL tab contains the Sulpher query I was editing previously." Fixed:
  each mode now keeps its own document, swapped via a full
  `EditorView.setState()` on switch rather than an incremental
  reconfigure.
- **Investigated, not confirmed fixed:** Sulpher syntax highlighting
  reportedly not appearing. Traced the full extension chain and found
  nothing obviously wrong; the mode-switch fix above may resolve it as
  a side effect (eliminates stale Compartment state) but this sandbox
  has no real browser to confirm visually. Recorded honestly in
  `KNOWN_ISSUES.md` rather than claimed fixed.
- **Nav redesign: the "Connections" top-nav link is gone, replaced
  with a proper brand/home link plus a real dark/light theme toggle.**
  Root cause of the "never highlighted, not a tab, not a recognisable
  UI element" report: `ActivePrefix: "/connections"` matched literally
  every page in the app (everything lives under `/connections/...`),
  so the link was permanently stuck in its own "active" state —
  providing no real feedback, which is indistinguishable from looking
  broken. The `dark:` Tailwind classes were already in place
  throughout from early on; `theme.js` (loads synchronously before the
  stylesheet, to avoid a flash of the wrong theme) is the toggle
  mechanism itself, which had genuinely never been finished after
  being requested. Persists via `localStorage`, falls back to system
  preference.
- Nav tests rewritten to match — the old active-marking test was
  effectively untestable once Connections stopped being a nav-visible
  module; now tests a real registered module instead of the one that
  no longer applies.
- **Investigated at length, not reproduced:** entities.go's Show/
  EditForm/Delete/DeleteConfirm and the ref-link generation in both
  the edit form and list preview were all tested directly, including
  with xolu's v2 API surface genuinely enabled — none produced
  `XOLU-ST004`. Needs the exact URL from the browser address bar when
  the error appears to pin down further.

## [0.6.13] — 2026-08-04

- **Process correction, not new work: T-11 (grid editor) closed.** The
  actual code — `internal/ui/grid.go`, `grid_test.go`,
  `web/static/js/grid-editor.js` — was built, tested (11 tests), and
  verified end-to-end against real xolu in an earlier pass this
  session, but the closure procedure never ran at the time; it landed
  silently inside a larger response without its own checkpoint. Caught
  while reviewing the register before continuing further work.
  Re-verified fresh before closing, not just trusted from the earlier
  record: grid page loads with real vendored assets, `grid-data` GET
  reflects real seeded data, `grid-data` POST genuinely `Patch`es
  (`_version` incremented, value actually changed on the real server).

## [0.6.12] — 2026-08-04

- **T-12 closed: query editor, all three modes (OQL, Sulpher, REST).**
  CodeMirror 6 vendored with three official language packages — MSSQL
  dialect for OQL (a T-SQL subset by construction), real ANTLR4 Cypher
  grammar for Sulpher (near-exactly openCypher9), JSON for REST request
  bodies — one self-contained 1.2MB bundle, Lit shell around it matching
  the same architecture as the grid editor. `internal/ui/query.go`
  dispatches to `Client.OQL`, `Client.GraphQuery` (not the deprecated
  `Sulpher` alias), and `Client.Raw` — the last of these genuinely
  unblocked by xolu v0.25.0, not previously possible.
- 10 Go handler tests plus JS syntax/bundle validation. Two real path
  mistakes caught before they shipped: guessed `/api/v1/oql` instead of
  the actual `/oql/query`, and `GraphQuery`'s `maxDepth` parameter
  (missed on first read of the signature — `0` uses xolu's own server
  default, confirmed from its doc comment rather than guessed).
- Verified end-to-end against real xolu 0.26.0, all three modes: OQL
  (`SELECT * FROM users WHERE age > 20`, real rows back with correct
  types), Sulpher (`MATCH (n) RETURN n LIMIT 5` against real graph
  data), and REST (`GET /api/v1/entities` returning real `ListEntities`
  data through `Raw`) — including confirming a real 400 from an
  invalid path came back as data with `statusCode` set, not a Go error,
  which is the entire point of an escape-hatch REST mode.

## [0.6.11] — 2026-08-04

- **xolu upgraded to v0.26.0.** Adds the FSM definition write methods
  requested in `docs/xolu-requests-fsm-def.md` —
  `CreateMachineDef`/`ReplaceMachineDef`/`DeleteMachineDef`/
  `ValidateMachineDef` — confirmed directly against the real source,
  matching the request closely. Bonus: `analysis` now comes back as a
  typed `MachineDefAnalysis` struct (reachability, determinism, cycles,
  warnings) on all four, and `GetMachineDef` gained a
  `ParsedAnalysis()` method. Two behavioral notes worth keeping for
  whoever builds against these: `ReplaceMachineDef` affects future
  machine creation only, no retroactive effect on already-running
  machines; `DeleteMachineDef` has no reference check at all.
- **Integrity of the xolu checkout explicitly re-verified before this
  upgrade, at Horacio's request** — every one of the 800 files in the
  v0.25.0 checkout checked against its own original `MANIFEST.sha256`:
  zero mismatches, zero missing files, the only unlisted file being the
  manifest itself (expected). No modifications had been made.
- `internal/xoluext/fsmdef.go` — the hand-rolled raw-HTTP workaround
  for these same four operations, built before the official client had
  them — deleted. Zero callers anywhere in the codebase (confirmed by
  grep before removing it); this was xoluman's own temporary code, not
  something the separately-tracked FSM feature work had started
  depending on. T-14's tracking entry updated to point at the real
  client methods instead, and to record the two behavioral notes above,
  since that work is being done on its own track and this update exists
  so whoever picks it up next sees the current, correct state.
- Fully backward compatible — full suite green against v0.26.0 with no
  code changes needed elsewhere.

## [0.6.10] — 2026-08-04

- **T-09 closed: blob browser.** `internal/blobfs` presents xolu's flat
  blob key store as a navigable folder hierarchy — a xoluman-only
  presentation convention (`:` as an in-key delimiter, since xolu keys
  reject `/` and `\`), never anything xolu itself knows about. Empty
  folders are backed by real `xoluman_blob_folder` entities (schema-
  less, same established pattern as `xoluman_field_meta`), reconciled
  against the real blob scan on every browse: a blob-implied folder
  with no matching entity gets one materialized; an implicit,
  now-blobless entity with no child folders gets garbage-collected;
  explicit folders persist regardless of contents.
- UI: browse with breadcrumbs, upload (streamed straight to `BlobPut`,
  no server-side temp file), download (streamed straight from
  `BlobGet`), delete a file, create an explicit empty folder, delete an
  empty folder — refused with `409` if it turns out not to be empty by
  the time the click lands, re-checked server-side rather than trusted
  from a possibly-stale listing.
- Routing note: Go's `{path...}` wildcard is a full-suffix match, so a
  sibling route like `.../download` under the same wildcard tree isn't
  expressible — only the browse view sits on the wildcard path; every
  other action lives on its own fixed route with the target key/parent
  path carried as form data instead.
- 17 tests in `blobfs` (90.2% coverage) against a genuinely stateful
  fake xolu (real blob-key prefix matching and real entity CRUD — a
  call-by-call mock can't honestly represent the reconciliation logic's
  actual interdependent behavior), 9 more for the UI handlers. A real
  bug caught in the test fixture itself, not the implementation:
  `make([]byte, r.ContentLength)` panics when `ContentLength` is `-1`
  (unknown length) — fixed to use `io.ReadAll`.
- **Another real environment gotcha found and recorded, same category
  as `XOLU_API_V2_ENABLED`:** blob storage is disabled by default too
  — needs `XOLU_BLOB_ENABLED=true`, otherwise every blob call fails
  with a 501 (correctly coded, at least, unlike the v2 gate's plain
  404).
- Verified end-to-end against real xolu 0.25.0 with blob storage
  actually enabled: uploaded a real file into a nested path, confirmed
  the implied folder was genuinely materialized as an entity (not just
  shown in the UI), downloaded the file back and got the exact original
  bytes, confirmed the real stored xolu key is colon-delimited, created
  and confirmed an explicit empty folder survives having no blobs, then
  deleted the file and confirmed the implicit folder chain was
  correctly garbage-collected on the next browse while the explicit
  folder was not.

## [0.6.9] — 2026-08-04

- **T-20 closed: schema promotion, a new entity-browser feature.** A
  "Promote to schema" link on every schemaless row in the entity type
  list opens a preview (`GetSchemaSuggestion` — sampled rows, per-field
  inferred type/coverage/confidence, suggested enums, no side effects)
  with the suggested schema in an editable textarea. Two submit paths:
  strict (validates every existing row first, only migrates if all
  pass, atomically — rejection renders which rows failed and why, and
  is a normal outcome, not an error page) and flex (fast, synchronous,
  explicitly labeled as not migrating pre-existing rows, with the
  server's own warning surfaced verbatim when it applies).
- 6 new tests, full suite green.
- Verified end-to-end against real xolu 0.25.0: seeded a schemaless
  type with 5 rows, previewed the real inferred suggestion, promoted
  strict, confirmed the schema is now genuinely registered
  (`GET /api/v1/schema/leads` 200, previously 404) and all 5 rows
  survived the migration and are still listable.

## [0.6.8] — 2026-08-04

- **T-16 closed: schema-less entities can now be created and edited
  through the generic form, not just browsed.** `NewForm`/`Create`/
  `EditForm`/`Update` all fall back to inferring fields from real data
  when `GetEntitySchema` 404s — `EditForm`/`Update` infer from the
  specific row in play (no extra round trip for `EditForm`, which
  already had the entity loaded; one extra `Get` for `Update`, which
  didn't); `NewForm`/`Create` infer from any one existing row via
  `List`. A genuinely empty, schema-less type (no schema, no data at
  all) shows a clear message instead of a meaningless empty form or a
  raw error. A real server error on the schema fetch still surfaces
  normally throughout — only a 404 degrades to inference.
- 12 new tests, full suite green.
- Verified end-to-end against real xolu 0.25.0: empty schema-less type
  → clear message; seeded one row → form correctly infers `label`/
  `priority`/`enabled`; created a second row through the real form,
  both landed with correct types (`priority` as a number, not a
  string); edited and updated the first row, correctly re-typed and
  persisted (`_version` incremented, confirming a real write).

## [0.6.7] — 2026-08-04

- **xolu upgraded to v0.25.0.** The xolu team's response to the
  standing request (T-13, closed) — four of six original items
  delivered (blob client methods, async tenant-scoped `Export`, `Raw`,
  `DefineEntitySchema`), plus two unrequested bonus items
  (`ListEntities`, schema promotion). Every claim in their delivery
  letter was checked directly against the v0.25.0 source before being
  trusted, not taken at face value — all held up exactly as described.
  Two items from the original request remain open, re-filed as T-21:
  `Client.Health()` still doesn't apply auth (confirmed unchanged),
  and the minor `FieldDef.Type` doc inconsistency.
- **T-17 closed: the entity-type discovery gap (T-16) is now properly
  fixed, not just worked around.** `ListEntities` lists every entity
  type with actual data — schemaless or not — with row counts and
  schema status, replacing the jump-by-name-only workaround from
  v0.6.5. The jump form stays as a fast path for a known name, but
  discovery itself no longer misses anything. Verified end-to-end
  against real xolu 0.25.0: a schema-registered type and a genuinely
  schemaless type, created directly against the server, both appear
  correctly with accurate counts and schema status in the same list.
- Filed for follow-up, not built this pass: the blob browser (T-09) and REST console (T-12) are now genuinely unblocked by the new
  `Blob*`/`Raw` methods; schema promotion (`GetSchemaSuggestion`/
  `PromoteFlex`/`PromoteStrict`) is a natural new entity-browser
  feature once a schemaless type is visible in the list; `Health()`'s
  auth gap re-filed standalone (T-21) since it's now the only thing
  left open from the original request.
- **Register cleanup, same pass:** T-02 and T-03 closed — the client
  methods they asked for (`Export`, `Raw`) now exist, delivered by the
  xolu team, not xoluman code; the actual UI features that need them
  are separately tracked (T-22 for backup/export, T-12 for the REST
  console). T-18 and T-19, filed right after the letter arrived,
  turned out to duplicate unblocking already captured in T-09 and T-12
  themselves — closed as merged back in rather than left as second
  open items for the same work.

## [0.6.6] — 2026-08-03

- Connection testing now works *before* saving — a "Test before saving"
  button in the New Connection modal, submitting the form's current
  field values directly (no store interaction at all) rather than
  requiring save-then-test-then-delete-if-wrong. `Create` and the new
  `TestUnsaved` handler share one `connectionFromForm` helper so the
  two never parse the submitted fields differently.
- Verified end-to-end against real xolu: reachable, unreachable, and
  the one guarantee that actually matters here — confirmed nothing
  gets persisted to the connection store either way.
- 5 new tests, full suite green.

## [0.6.5] — 2026-08-03

- **Real bug, from a real user report: schema-less entity types were
  completely invisible in the entity browser.** Traced to a wrong
  assumption baked into the browser since T-07/T-10 first shipped —
  that every entity type has a registered schema. It doesn't have to:
  xolu lets you create and read entity data with zero schema ever
  registered, but `GET /api/v1/schemas` only reflects *registered*
  schemas, and `GET /api/v1/schema/{entity}` 404s for an unregistered
  type. Every entity page called `GetEntitySchema` first and treated
  any failure as fatal — so a schema-less type's data was real,
  reachable directly via curl, and completely unbrowsable through
  xoluman.
- Fixed: `Show` now infers preview columns from the fetched rows' own
  JSON value types when the schema fetch 404s (`inferFieldsFromEntities`
  — union of keys across returned rows, type from each value's JSON
  shape), rather than failing the page. A genuine server error on the
  schema fetch still surfaces normally — only a 404 degrades.
- The entity-type list page gained a "browse by name" jump — discovery
  via `/schemas` is incomplete by xolu's own design (it can only ever
  list *registered* schemas), so a client-side fix can't make it
  complete; typing a known type name bypasses discovery entirely, since
  `List` itself doesn't need a schema.
- **Not yet fixed, tracked as T-16**: `EditForm`/`Update`/`NewForm`/
  `Create` still assume a schema — a schema-less type can be browsed
  now but not yet edited or created through the generic form.
- Verified end-to-end against real xolu: created a genuinely
  schema-less entity, confirmed it was invisible to the old code path,
  confirmed the fix makes it visible and correctly rendered.
- 8 new tests, full suite green.

## [0.6.4] — 2026-08-03

- **T-15 (partial)** — REF field navigation and listbox/select fields,
  from an explicit difficulty assessment. Neither needs a new xolu API.
- `internal/fieldmeta`: listbox/select field configuration stored as
  `xoluman_field_meta` entity documents — schema-less, the same
  established pattern as T-09's blob folders, not a new xolu primitive.
  Static option lists and ref-sourced ones (options looked up from
  another entity's own options-shaped field) both supported.
- `formengine.RenderFields` refactored from four positional arguments to
  a `RenderOptions` struct (`Values`/`Errors`/`ReadOnly`/`FieldOptions`/
  `RefLinks`) — it needed two more capabilities and four positional
  params was already the practical limit. Only one real call site
  existed, so low risk; every entity form handler (`NewForm`, `Create`,
  `EditForm`, `Update`) updated. Select rendering takes priority over
  type-based dispatch (a boolean field with configured options renders
  as a dropdown, not a checkbox); required fields skip the empty
  leading choice.
- `internal/ui/refs.go`: `resolveFormOptions` builds ref navigation
  links from `EntitySchema.Refs` (confirmed this already gives the
  target entity type directly, no guessing needed) for any *set* ref
  field, with a name/title/label heuristic for the link text — falling
  back to `type #id` even when the fetch itself fails, so a broken
  reference stays visible and clickable rather than vanishing.
- A real bug caught in my own test, not the implementation: assumed
  minty would render `selected` after `value` in an `<option>` tag;
  minty sorts attributes alphabetically, so it doesn't. Fixed to check
  the whole tag rather than assume an order.
- Coverage: 88.9% (fieldmeta), full suite green under `-race`.

**Explicitly postponed, not started (T-15's tracking entry has the
full detail):**
1. Ref links in the entity *list* preview — cheap, just not wired in yet.
2. The one-level side-panel hierarchy view — `resolveFormOptions`
   already computes what it needs; needs a rendering pass.
3. Grid editor's Tabulator `list`-editor integration for select fields
   — config shape already verified against the real vendored source,
   `buildGridColumns` doesn't consume `fieldmeta` yet.
4. End-to-end verification against real xolu — everything above is
   unit-tested only this pass, unlike T-05/T-10/T-11's grid API.
5. Full nested inline *editing* of a linked document within the parent
   form — assessed High difficulty, recommended against for v1 (real
   complexity around nested form state and save semantics). Not
   started, no plan to start without a separate design pass.

## [0.6.3] — 2026-08-03

- `internal/xoluext/fsmdef.go`: FSM definition write methods
  (`CreateMachineDef`, `ReplaceMachineDef`, `DeleteMachineDef`,
  `ValidateMachineDef`) — calling xolu's already-documented
  `/api/v2/fsm/def` REST endpoints directly, since the official
  `xolu/pkg/client` doesn't wrap them yet (requested, T-13,
  `docs/xolu-requests-fsm-def.md`). Not a modification to xolu — this
  is xoluman consuming xolu's own public API, entirely within
  xoluman's own repository. Deletable in one shot once the official
  client methods land.
- 91.4% coverage, all four operations plus auth-header/tenant-URL
  construction and both error-decoding paths (structured XOLU error
  envelope and raw-body fallback) unit tested.
- Verified end-to-end against a real running xolu binary — full
  lifecycle (validate → create → get via the *official* client,
  proving genuine round-trip through real storage → replace → delete
  → confirmed gone). Caught three real things worth knowing, now
  recorded in `docs/KNOWN_ISSUES.md`: xolu's `/api/v2` surface is
  disabled unless the server sets `XOLU_API_V2_ENABLED=true` (404s
  with plain "page not found," not an XOLU-coded error, otherwise);
  guard expressions are T-SQL syntax (`=`, not `==`); `Determinism` is
  required with exactly three valid values, and a transition's
  `Output` must be pre-declared in `OutputAlphabet`.

## [0.6.2] — 2026-08-03

- `internal/modules.Module` gained `MountRoutes func(*http.ServeMux)`,
  matching Seam AMS's actual module pattern — T-08 had only ported the
  nav-registration piece; every route was still hand-wired centrally in
  `internal/server/server.go`. `RegisterConnectionsModule`/
  `RegisterEntitiesModule` now own their own route tables;
  `server.New` shrinks to building the registry, calling `MountAll`,
  and the two things that aren't modules (static asset serving, the
  root redirect).
- Real design constraint solved, not glossed over: the shared,
  `init()`-populated registry `internal/ui` uses for nav rendering
  can't also be used for route mounting — `server.New` is called once
  per real process but dozens of times across the test suite, and
  `http.ServeMux` panics on a duplicate pattern registration. Nav
  registration stays `init()`-based (store-independent, safe to run
  once); route mounting builds a fresh registry per `server.New` call.
  Verified directly: the full suite (which calls `server.New` many
  times) passes clean under `-race`, and a targeted 5-run repeat of the
  connections→entities flow against real xolu binaries showed no panics
  or duplicate-route errors.
- No user-visible behaviour change — confirmed via the same real
  end-to-end sequence already used to verify T-10 (create a connection,
  browse its entities) against real running xolu/xoluman binaries.

## [0.6.1] — 2026-08-03

- T-08 closed: `internal/modules` — a self-registering module registry
  (adapted from Seam AMS's own `internal/modules`, without Seam's
  role-based `VisibleTo(role)` gating, which xoluman has no
  multi-user/role model to need), replacing the hardcoded nav list in
  `internal/ui/layout.go`. `Registry.Visible(predicate)` exists as the
  hook a future visibility layer could use, unbuilt for now. Connections
  registers itself via `init()` in `connections.go` — the pattern future
  modules (graph editor, FSM editor) follow when they land. No
  user-visible change yet with only one module registered; this is
  infrastructure for when there's more than one, not a new capability.

## [0.6.0] — 2026-08-03

- **T-05 closed — entity import from CSV/JSON files.** Two-phase
  upload → preview → confirm flow: `internal/importer` parses the
  upload (rows independent — no native xolu bulk-import endpoint exists,
  confirmed in T-05's own tracking, so one row's failure never affects
  another's), a short-lived in-memory session (random ID, 30-minute
  expiry, single-use, lazily swept — not a background goroutine) bridges
  preview and confirm without re-uploading the file or stuffing
  arbitrarily many rows into hidden form fields. Reuses T-07/T-05's own
  `formengine.ParseFormValues` for CSV type coercion rather than
  re-implementing it, since a CSV cell and a submitted form field are
  the same shape once you're past reading the file.
- **A real bug caught and fixed before shipping, not discovered by a
  test written to match broken behaviour:** CSV boolean columns were
  read via presence, not value — inherited from `ParseFormValues`'s
  correct-for-HTML-checkboxes logic (a present form key means checked,
  since browsers omit unchecked boxes from submissions entirely), which
  is wrong for a file column that can legitimately be present with an
  empty or literal `"false"` cell. Every boolean column would have
  silently become `true` the moment it appeared in the import file at
  all. Fixed with dedicated value-based CSV boolean parsing
  (`true`/`1`/`yes`/`on` vs `false`/`0`/`no`/`off`/empty), not patched
  by loosening the test.
- Verified end-to-end against a real xolu instance: uploaded a real CSV
  with a mix of valid, partially-empty, and invalid rows; confirmed only
  the valid rows landed, with correct type coercion (integer, boolean)
  and correct omission of an empty optional cell (not coerced to a
  zero/false default).
- 95.2%/96.3%/78.8% coverage (importer/server/ui), full suite green
  under `-race`.

## [0.5.0] — 2026-08-03

- **T-10 closed — the entity browser (both halves now done).**
  `internal/ui/entities.go`: entity type list, paginated entity list
  with a bounded data preview (up to 4 non-object/array fields as
  columns, long strings truncated — a generic browser across arbitrary
  schemas needs a bound, full field access is the edit form's job),
  create/edit forms wrapping T-07's `formengine.RenderFields`, delete
  confirmation via the shared modal.
- `internal/formengine.ParseFormValues`: the inverse of `RenderFields`
  — submitted form values back to typed data per field, per-field
  errors rather than an all-or-nothing failure, the checkbox
  absent-means-false browser behaviour handled explicitly, decimal
  fields kept as exact strings end to end (render to submit, never
  round-tripped through float64).
- Caught and fixed during testing, not assumed correct: the first pass
  at the entity list table showed bare IDs only, no data — genuinely
  useless for identifying which row is which without opening each one.
  Fixed before considering this done, not shipped and revisited later.
- Verified end-to-end against a real xolu instance: registered a real
  schema, created real rows, browsed/edited/pre-populated them through
  the actual running UI — not just the mocked test suite.
- 95.8%/78.5% coverage (server/ui), full suite green under `-race`.

So, concretely: xoluman can now genuinely browse and edit arbitrary
entity data on a connected xolu instance, in a real browser. That's the
milestone this whole T-06→T-07→T-10 chain was for.

## [0.4.1] — 2026-08-03

- **Correction:** T-01 (blob client methods) was implemented and closed
  directly against Horacio's local xolu checkout — wrong. xoluman does
  not modify xolu; changes to xolu are requests to the xolu team, not
  code this project writes into someone else's repository, regardless
  of what a `go.mod replace` directive makes locally buildable. Caught,
  the code was discarded, and the mistake is recorded as a correction
  note in `docs/RESOLVED.md` rather than rewritten out of history.
  Recorded as a foundational decision in `docs/KNOWN_ISSUES.md`.
- `docs/xolu-requests.md`: a plain-language request document to the
  xolu team (T-13), covering everything xoluman actually needs from
  xolu — reviewed comprehensively, not just the blob/export work
  already in flight. Includes two findings from that review: a
  documentation bug in `EXPORT_API.md` (wrong manifest field names,
  a `graph_files` key that doesn't exist) and a functional gap in
  already-shipped xoluman functionality (`Client.Health()` never
  applies the configured auth header, so "Test connection" can't
  actually validate a credential, only server reachability).
- T-02, T-03 reframed as blocked on the xolu team's response (T-13)
  rather than xoluman-implementable.

## [0.4.0] — 2026-08-03

- Blob primitive methods added to `xolu/pkg/client` (T-01, closed — see
  `docs/RESOLVED.md`): `BlobPut`, `BlobGet`, `BlobHead`, `BlobDelete`,
  `BlobList`, `BlobUsage`. Request/response shapes verified directly
  against `pkg/server/blob_handlers.go`'s exact JSON tags, not just
  `BLOB_API.md`'s prose (which omits `blobPutResponse`'s `size` field).
  Full xolu `pkg/client` suite still green; whole-repo `go build`
  confirmed no wider breakage. This is genuinely upstream xolu work,
  done against the local checkpoint xoluman's `go.mod` replaces —
  **not yet given a real xolu T-number or release**, since that
  checkpoint (v0.24.3) predates T-141 known to exist upstream at
  v0.24.4; assigning a number here risked colliding with whatever's
  actually next in the live xolu register. Needs proper registration
  and release cycling when reconciled with the real xolu working copy.

## [0.3.0] — 2026-08-03

- `internal/formengine`: the schema-driven generic form renderer (T-07,
  closed — see `docs/RESOLVED.md`). Renders flat input rows from
  `client.FieldDef` in schema order — text/email/url by format, number
  with correct step for integer vs. number, decimal deliberately kept as
  text (never `<input type="number">`, which coerces through float64 and
  loses exactly the precision xolu's decimal type exists to preserve),
  checkbox for boolean, datetime-local for date-time/timestamp, a JSON
  textarea fallback for object/array (flat fields only — no nested
  sub-forms), ref fields editable by target ID. Inline validation errors,
  per-field read-only/disabled. Deliberately not Seam's formengine — no
  tabs, no `x-seam-relation`, no visibility rules. 100% statement
  coverage, no server required to test (same pattern as `connstore`).
- Sorted the full open register by dependency order, then priority,
  before starting this work — T-07 was the first ready item with no
  unmet prerequisites.

## [0.2.3] — 2026-08-03

- Reversed course on styling: adopted Seam's actual Tailwind CSS build
  (package.json/tailwind.config.js/scripts/tailwind.input.css mirroring
  Seam's own setup exactly, `make css` / `npm run css` compiles the
  embedded stylesheet) instead of the custom inline CSS built in 0.2.0,
  which reasoned incorrectly that Tailwind was "bloat" Seam's design
  system didn't need. All of `internal/ui` migrated to Seam's own
  button/table/form Tailwind class conventions. Compiled output is
  committed and embedded via `go:embed` — never fetched at runtime, same
  disconnected-operation guarantee as 0.2.2, now via the real system
  instead of a substitute.

## [0.2.2] — 2026-08-03

- Fixed: htmx was being loaded from a CDN, directly violating xoluman's
  disconnected-operation requirement (it manages xolu instances that may
  be on air-gapped local networks). Vendored locally
  (`web/static/vendor/htmx@1.9.10.min.js`) and embedded into the binary,
  same as `modal.js`. Added `TestPage_NoExternalCDNReferences` so this
  can't silently regress again.
- Corrected a mischaracterization in `docs/KNOWN_ISSUES.md`: vendoring
  Tailwind/htmx/Lit in Seam was never about avoiding "bloat" — it's
  about the whole application working without internet access at all.
  Recorded as a foundational, project-wide decision, not a page-level
  detail.
- Filed T-11: bulk/grid data editing (as distinct from T-07's
  single-entity form) via vendored Tabulator (MIT, vanilla JS — Glide
  Data Grid was ruled out, confirmed React-only with no vanilla build)
  wrapped in a Lit shell, matching the architecture already established
  for the FSM editor and planned for the graph editor.

## [0.2.1] — 2026-08-03

- Correction: the connection management UI shipped in 0.2.0 didn't
  actually carry over the modal/listing conventions agreed earlier in
  the project — full-page navigation instead of a modal, browser
  `confirm()` instead of a modal confirmation, one-off table markup
  instead of a reusable component. Fixed: `internal/ui/listing.go`
  (trimmed adaptation of Seam's listing engine, no Tailwind/Material
  Icons dependency), `web/static/js/modal.js` (from-scratch `XModal`
  controller, same API shape as Seam's `SeamModal`, much smaller),
  embedded via `web/embed.go`. Connection management refactored onto
  both. Re-verified end-to-end against the real binary.

## [0.2.0] — 2026-08-03

- `cmd/xoluman`: the actual binary entrypoint. Loads settings, builds
  the configured `connstore` backend, starts the HTTP server.
- `internal/server`: route table on the standard library's
  method+pattern `http.ServeMux` (Go 1.22+) — no router dependency.
- `internal/ui`: minty-based page shell and nav; connection
  list/add/delete pages; an htmx "Test connection" button backed by
  `client.Health`.
- `internal/xoluext.BuildClient`: stored `Connection` →
  `*xolu/pkg/client.Client`, auth mode and tenant mapping verified
  against actual outgoing requests.
- Verified end-to-end against a real running xolu instance, not just
  the test suite: connection management (add, list, test, delete) works
  in a real browser round-trip against real xolu. Reachable/unreachable
  states confirmed by actually killing the upstream mid-test.
- Partially closes T-10 (◐) — the connection management half is done
  and tested (100%/90.2%/100% coverage: server/ui/xoluext); the entity
  browser half is blocked on T-07, not yet started.

## [0.1.0] — 2026-08-03

- Project scaffolded: repo layout, repoman tooling installed and
  verified (selftest: 18/18 green), tracking-document taxonomy in
  place (`TRACKING.md`, `RESOLVED.md`, `KNOWN_ISSUES.md`).
- `internal/config`: app configuration directory resolution and
  `settings.json` (secret storage backend selection, `file` or
  `keyring`, made once at first run).
- `internal/connstore`: the `Store` interface and both backends —
  `FileBackend` (plaintext JSON, `0600`, atomic writes) and
  `KeyringBackend` (metadata in JSON, tokens in the OS keyring via
  `github.com/zalando/go-keyring`, never both in the same place).
  86.0%/78.0% statement coverage (connstore/config). Closes T-06 — see
  `docs/RESOLVED.md`. T-04 (keyring backend) ships implemented and
  behaviourally tested against an in-memory mock, but stays open at
  partial (◐) pending a real-OS-keyring round-trip that this sandbox
  cannot run — see the dormant guard in `docs/KNOWN_ISSUES.md`.
- repoman fix (in-repo copy, not yet reported upstream): `register.py
  close` could not perform a brand-new repository's first-ever closure
  — it required an existing `## ` entry in `RESOLVED.md` to insert
  before, which no fresh repository has. Now falls back to appending
  after the intro prose when none exists.
