# Changelog

All notable changes to xoluman are recorded here.

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
