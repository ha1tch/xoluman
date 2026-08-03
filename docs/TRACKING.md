Version: 0.6.4
Last reviewed: 2026-08-03

# xoluman — Live Register

Open, actionable items only. Closed items move to `RESOLVED.md` in full,
per the closure procedure. See `docs/KNOWN_ISSUES.md` for intentional
limits and recorded decisions rather than open work.

## Status table

| ID | Summary | Theme | Priority | Status | Blocks/after |
|----|---------|-------|----------|--------|---------------|
| T-02 | Add Export method to `xolu/pkg/client` (`GET /api/v1/export`) | xolu-client-ext | P2 | ☐ | Blocks: backup feature |
| T-03 | Add a minimal Raw request method to `xolu/pkg/client` for the REST query console | xolu-client-ext | P3 | ☐ | Blocks: raw REST query runner |
| T-04 | Implement `ConnectionStore` keyring backend | connstore | P1 | ◐ | After: T-06 (closed, v0.1.0) |
| T-09 | Blob browser: virtual hierarchy over the flat key store, backed by a `xoluman_blob_folder` entity | blob-browser | P2 | ☐ | After: T-01 |
| T-11 | Bulk/grid data editing: vendored Tabulator (vanilla JS) wrapped in a Lit shell | grid-editor | P3 | ☐ | After: T-07 (single-entity form ships first) |
| T-12 | Query editor: CodeMirror 6 for all three query modes (OQL/Sulpher/REST), Lit shell | query-editor | P2 | ☐ | After: T-10 (UI shell) |
| T-13 | External request filed with the xolu team: client-library gaps, Health() auth gap, schema registration | xolu-client-ext | P1 | ☐ | Blocks: T-02, T-03, T-09; blocked on xolu team response |
| T-14 | FSM def module: list + detailed read-only viewer, real Module architecture | fsm-def-module | P2 | ☐ | After: none, self-contained |
| T-15 | REF field navigation + listbox/select fields via xoluman_field_meta | ref-and-listbox | P2 | ◐ | After: none, self-contained. No new xolu API needed for anything in scope. |

## Detail

### T-02. Add Export method to `xolu/pkg/client`

Theme: xolu-client-ext · Priority: **P2** · Status: ☐ · Blocks/after: After T-13 (xolu team response); blocks backup feature

`GET /api/v1/export` streams a zip (manifest + database file + optional
`graph.json`). Needs a streaming-friendly client method (`io.Writer`
target, not a buffered `[]byte` return, given arbitrary DB size per
`EXPORT_API.md`'s own caveat about streaming without a temp file).

**Reframed 2026-08-03:** not xoluman-implementable — changes to
`xolu/pkg/client` are requests to the xolu team (see T-13,
`docs/xolu-requests.md`), not something this project writes into
Horacio's local xolu checkout itself. While reviewing this for the
request doc, found `EXPORT_API.md` itself is stale: it documents the
database file as `entities.db` / manifest key `entities_file`, and a
`graph_files` array key that doesn't exist. Worth flagging to the xolu
team as a doc bug regardless of when/whether the client method lands.

### T-03. Add a minimal Raw request method to `xolu/pkg/client`

Theme: xolu-client-ext · Priority: **P3** · Status: ☐ · Blocks/after: After T-13 (xolu team response); blocks raw REST query console

`Client.do`/`doURL` are unexported. The REST console needs to issue
arbitrary method+path+body requests using the connection's already-
configured auth, so a small public `Raw(ctx, method, path, body)
(status int, body []byte, err error)` on the client would cover it —
same reasoning as T-01/T-02, keep auth/retry logic in one place.

**Reframed 2026-08-03:** not xoluman-implementable, same correction as
T-02 — this is a request to the xolu team (T-13,
`docs/xolu-requests.md`), not something built directly into Horacio's
local checkout.

### T-04. `ConnectionStore` keyring backend

Theme: connstore · Priority: **P1** · Status: ◐ · Blocks/after: After T-06 (closed, v0.1.0)

Implemented against `github.com/zalando/go-keyring` v0.2.8:
`KeyringBackend` (`internal/connstore/keyring_backend.go`) stores
connection metadata in `connections-meta.json` and tokens in the OS
keyring under service `xoluman`, keyed by connection name. Metadata
never contains the token — verified by
`TestKeyringBackend_MetadataFileNeverContainsToken`, which asserts the
raw file bytes don't contain a known-secret value.

**Shipped:** the full behavioural test suite runs against
`keyring.MockInit()`'s in-memory provider (no OS keyring service exists
in the sandbox this was built in) — upsert semantics, `ErrNotFound`,
`ErrTokenUnavailable` when metadata exists but the keyring entry doesn't
(simulating external removal via the OS's own keyring UI), and both
rollback directions in `Save` (keyring write fails → no metadata written;
metadata write fails after a successful keyring write → the keyring
entry is rolled back). Confirmed against a genuinely absent keyring
service too: building with `-tags keyring_live` in this sandbox produces
a clear, informative failure (`exec: "dbus-launch": executable file not
found in $PATH`), not a panic or hang — exactly the failure mode the
dormant guard below exists to catch on a machine that's supposed to have
one.

**Outstanding — cannot be verified in this sandbox:** an actual
round-trip against a real OS keyring service. `-tags keyring_live`
swaps out `TestMain`'s mock initialization (`testmain_mock_test.go` /
`testmain_live_test.go`, split by build tag) so the *same* test suite in
`keyring_backend_test.go` becomes a genuine round-trip with no
duplicated test logic — it just hasn't been run against a real provider
yet. See the dormant-guard entry in `KNOWN_ISSUES.md` for the exact
invocation. T-04 closes once that's run and reported.

### T-09. Blob browser: virtual hierarchy over the flat key store

Theme: blob-browser · Priority: **P2** · Status: ☐ · Blocks/after: After T-01

xolu's blob keys cannot contain `/` — confirmed this is enforced even
on the S3-compatible surface (`pkg/server/blob_s3_handlers.go` calls
straight through to the same `blob.Store.Put`/`validateKey` as the
native endpoint), so no path in xolu accepts real hierarchical keys.
The hierarchy is therefore a xoluman-only presentation convention, not
anything xolu is aware of.

**Delimiter:** `:` (colon) used *within* an otherwise-flat key —
`photos:2026:vacation.jpg` — translated to `photos / 2026 /
vacation.jpg` only at the xoluman UI boundary. Never written to xolu
with any other meaning; the stored key is still one opaque flat string
as far as xolu is concerned.

**Listing a folder:** `GET /api/v1/blob?prefix=photos:2026:` (native,
already supported, no xolu changes needed) — split each returned key on
`:` after stripping the prefix, first remaining segment is one
directory level.

**Empty folders and drift, backed by a `xoluman_blob_folder` entity**
(confirmed with Horacio 2026-08-03 — uses xolu's own entity/REF
mechanism rather than a xoluman-local index, so it travels with the
target instance's own backup/export and is visible to any other client
of that instance, not just xoluman):

| Field | Type | Purpose |
|---|---|---|
| `name` | string | This level's own segment name only, not a full path |
| `parent` | `REF → xoluman_blob_folder`, nullable | Root folders have no parent; the REF is the graph edge, created automatically |
| `explicit` | bool | `true`: a person deliberately created an empty folder. `false`: xoluman auto-materialized it because blobs were observed under that prefix |

No `path` field — deliberately. Full path is reconstructed by walking
`parent` REFs on demand, so renaming a folder touches exactly one
entity's `name`, never a cascade of stored-path rewrites on every
descendant.

**Reconciliation (this is the actual "sync"):** browsing into a folder
runs the blob prefix scan and an entity query for `parent = this
folder` in parallel, then merges: a blob-scan segment with no matching
entity gets one materialized now (`explicit: false`) — this heals
folders created by anything that bypassed xoluman entirely (raw curl
against the blob API). An `explicit: false` entity that loses its last
blob and has no child folders is garbage-collected, since it only ever
existed as a side effect of content being there. An `explicit: true`
entity persists regardless of contents. The entity store is never the
sole source of truth for anything populated — the blob scan always is;
the entity only carries what the blob store structurally cannot: empty
folders, and the "a person meant this to exist" bit.

**Move/rename:** no native move endpoint on xolu; content is SHA-256
deduplicated, so moving a file is `Put` under the new key + `Delete`
the old key alias — cheap, no data actually re-copied. Moving a folder
means doing that for every blob under its prefix (no cheaper option
exists) plus updating the one folder entity's `parent`.

## grid-editor

### T-11. Bulk/grid data editing: vendored Tabulator (vanilla JS) wrapped in a Lit shell

Theme: grid-editor · Priority: P3 · Status: ☐ · Blocks/after: After: T-07 (single-entity form ships first)

Confirmed 2026-08-03: Glide Data Grid is React-only (peer dependency on React 16-19, no vanilla build — verified, not assumed) and was ruled out on that basis, matching the project's no-React rule. Tabulator (github.com/olifolkerd/tabulator, MIT licensed) is the alternative — genuinely vanilla JS, ships as a plain JS+CSS pair (dist/js/tabulator.min.js + dist/css/tabulator.min.css), vendorable exactly like htmx and Lit already are. Wrapped in a Lit shell per Horacio's suggestion, this reuses the same architecture already established for the FSM editor and planned for the graph editor: a Lit shell (toolbar/theming/lifecycle) around a swappable, framework-agnostic engine. All three of xoluman's richer embedded widgets (FSM editor, graph editor, grid editor) end up sharing one consistent pattern rather than three different ones. Distinct from T-07: T-07 is a single-entity schema-driven form; this is bulk editing across many rows/cells of one entity type at once (paste, keyboard nav, multi-cell selection) — a materially bigger feature, correctly sequenced after T-07 ships.

**Design pass, 2026-08-03:**

- **Additive, not a replacement.** The grid is a second view of an entity type, reached from a "Grid view" link on the existing paginated list (T-10) — not a replacement for it. The simple list+form flow stays the default for occasional single-row edits; the grid is specifically for editing many rows quickly (paste, fill-down, keyboard nav).
- **Scope: bulk edit only, not bulk create/delete.** Creating and deleting rows already have homes — the entity form (T-07) and import (T-05) for creation, the existing delete-confirm flow for removal. The grid's job is editing values across many existing rows at once. Keeping create/delete out of it avoids duplicating either.
- **Real gotcha caught at design time, not as a data-loss bug later: writes must be PATCH, never PUT.** The grid can't reasonably show every field of a wide schema as a spreadsheet column — some columns will be omitted for readability, the same way T-10's list preview caps at 4. If a save sent `Client.Update` (a full-document PUT), any field not shown as a grid column would be silently dropped from the document on save. `Client.Patch` (partial update, PATCH) is the only correct choice here — it only touches the fields actually present in what's sent, exactly matching what a partial-column grid needs. This must not be revisited casually later; it's the one design fact that would turn "edit conveniently" into "silently delete data."
- **Column set:** every non-object/array scalar field from the schema (unlike the list preview's 4-column cap, which exists for glanceability, not editability — a grid meant for bulk editing should show what there is to edit). Object/array fields excluded, same reasoning as everywhere else in xoluman: not suited to a spreadsheet cell, formengine's own edit form is where those live. Ref fields editable by raw target ID, same v1 limitation as elsewhere — no lookup-by-name picker yet.
- **Data transfer, client ↔ server:** a dedicated JSON endpoint per entity type (`GET .../grid-data`) that Tabulator's own remote-pagination mode (`ajaxURL` + `pagination: "remote"`) consumes directly — reusing `Client.List`'s existing pagination rather than loading an entire (potentially large) entity type into the browser at once. Saves batch through a matching `POST .../grid-data` accepting per-row partial changes, executing one `Patch` per row, independently — same "one row's failure doesn't affect another's" philosophy already established for import (T-05), reported back the same way.
- **Real limitation, stated plainly:** the Lit+Tabulator component itself is client-side interactive JavaScript — this sandbox has no real browser to click into, drag-select cells in, or paste into. What gets verified here is everything that can be: the JSON API endpoints against real xolu (fully Go-testable, same rigor as everything else), asset vendoring and serving, and that the page assembles and the script loads without error. Actual interactive grid behaviour — editing, keyboard nav, the save flow's real UX — needs Horacio's own hands, the same category of gap as T-04's live-keyring guard.

## query-editor

### T-12. Query editor: CodeMirror 6 for all three query modes (OQL/Sulpher/REST), Lit shell

Theme: query-editor · Priority: P2 · Status: ☐ · Blocks/after: After: T-10 (UI shell)

Confirmed 2026-08-03 with Horacio's precise language facts: OQL is a subset of T-SQL, Sulpher is almost exactly openCypher9. Shiki was the original candidate but is superseded — it's a highlighter only (codeToHtml, no cursor/typing/selection), not an editor, and requires vendoring the Oniguruma WASM binary alongside it. CodeMirror 6 (MIT, github.com/codemirror, genuinely vanilla JS core — 'import {EditorView, basicSetup} from "codemirror"', no framework needed) is the actual editable component, and purpose-built official language packages cover both DSLs better than Shiki's generic/community grammars would have: OQL uses @codemirror/lang-sql's built-in MSSQL dialect (confirmed via its changelog's explicit MSSQL keyword/builtin coverage, and docs on MSSQL-style bracket-quoted identifiers) -- safe to use the T-SQL dialect for a T-SQL subset, since anything OQL uses is valid T-SQL by construction, zero custom grammar work needed. Sulpher uses @neo4j-cypher/codemirror (Apache-2.0, framework-agnostic base package -- the React wrapper @neo4j-cypher/react-codemirror is separate and not needed), built on Neo4j's real ANTLR4 Cypher grammar plus semantic analysis rather than a regex-based TextMate approximation, and it ships autocompletion, linting, and formatting already, not just highlighting. REST query bodies use @codemirror/lang-json (official, MIT). Net effect: Shiki and its WASM dependency are dropped entirely -- CodeMirror alone, three official per-language packages, better accuracy on both real DSLs than the generic grammars Shiki would have supplied. Wrapped in a Lit shell, this is the fourth instance of the same architecture already used for the FSM editor, planned for the graph editor, and T-11's grid editor: a Lit shell (toolbar/theming/lifecycle) around a swappable, framework-agnostic engine -- one consistent pattern across every rich embedded widget in xoluman rather than four different ones.

## xolu-client-ext

### T-13. External request filed with the xolu team: client-library gaps, Health() auth gap, schema registration

Theme: xolu-client-ext · Priority: P1 · Status: ☐ · Blocks/after: Blocks: T-02, T-03, T-09; blocked on xolu team response

Filed 2026-08-03 as docs/xolu-requests.md, a plain-language request document, not code. Corrects the mistake recorded in T-01's RESOLVED.md entry: xolu/pkg/client's blob and export methods were written directly into Horacio's local xolu checkout without asking, then discarded at his instruction once caught. xoluman does not modify xolu directly regardless of the go.mod replace directive making a local copy buildable -- changes to xolu are requests to the xolu team. See docs/KNOWN_ISSUES.md's recorded decision. The request document covers, reviewed comprehensively rather than just the blob/export items already in flight: Blob primitive client methods (blocks T-09 and the originally-scoped blob browser feature), a streaming Export method (blocks the backup feature; also flags EXPORT_API.md's stale manifest-shape documentation), a minimal Raw request method (blocks T-12's REST query console), a client wrapper for the existing but unwrapped POST /api/v1/schema/{entity} registration endpoint (needed for T-09's xoluman_blob_folder entity to get proper validation and to browse/edit correctly through xoluman's own generic data editor), and a functional correctness finding: Client.Health() never applies the configured auth header, confirmed by direct source inspection -- meaning xoluman's already-shipped 'Test connection' feature can only confirm the server is reachable, not that the stored token is actually valid. Closes when the xolu team responds; T-02/T-03/T-09 pick back up from whatever they decide.

**Second document filed 2026-08-03:** docs/xolu-requests-fsm-def.md -- a separate, focused request for the FSM definition write methods T-14 needs (CreateMachineDef/ReplaceMachineDef/DeleteMachineDef/ValidateMachineDef), split out from the main request rather than appended, since the xolu team was already mid-refinement on the /blob API and a tight, precisely-scoped second ask fit that timing better than folding into the larger document. Exact request/response shapes verified directly against pkg/server/v2_fsm_def_handlers.go, not inferred from docs.

## fsm-def-module

### T-14. FSM def module: list + detailed read-only viewer, real Module architecture

Theme: fsm-def-module · Priority: P2 · Status: ☐ · Blocks/after: After: none, self-contained

Prompted by Horacio noticing xoluman's module registry (T-08) only ported Seam's nav-registration piece, not the fuller pattern: Seam's actual modules.Module has MountRoutes func(*http.ServeMux) -- each module owns its own route registration, Registry.MountAll mounts everything in one call -- instead of every route being hand-wired centrally in internal/server/server.go the way connections.go and entities.go currently are. Extending modules.Module with MountRoutes and refactoring the existing two feature areas onto it, then building this new module the same way from the start, rather than adding a third inconsistent wiring style. Scope for the FSM def module itself, checked against the real xolu source rather than assumed: server-side has full CRUD (POST/GET/PUT/DELETE /api/v2/fsm/def, plus /fsm/def/validate) but pkg/client only wraps ListMachineDefs/GetMachineDef -- read-only, no create/replace/delete/validate methods exist to build a real editor against. Also checked MachineSpec/TransitionDef's actual wire shape (states carry a terminal flag; transitions carry guard/output/set, not just a label) against Seam's actual FSM canvas engine (seam-fsm-editor.js, genuinely Evan Wallace's code) -- confirmed the canvas only ever stores one free-text label per transition, no guard/output/set fields anywhere in it; the March-2026 fsm-toolkit feature request (docs in the Seam checkpoint) was for the data model only, the canvas UI was never extended to match. Reusing it as-is would silently collapse guard/output/set into one label field on save -- a real data-loss trap, not a cosmetic gap. Scoped down to what's honestly buildable now: a list + detailed read-only textual/tabular viewer (states with terminal flags, transitions with full from/input/to/guard/output/set, variables) -- represents the real data completely, no lossy conversion, and is genuinely useful on its own for understanding what an FSM actually does. The visual canvas (Wallace engine, read-only rendering or a properties-panel extension for real editing) and actual create/edit capability are separate, later work -- the latter blocked on client methods added to T-13.

## ref-and-listbox

### T-15. REF field navigation + listbox/select fields via xoluman_field_meta

Theme: ref-and-listbox · Priority: P2 · Status: ◐ · Blocks/after: After: none, self-contained. No new xolu API needed for anything in scope.

Two features from an explicit difficulty assessment (2026-08-03): REF fields as navigable links (clickable, resolved-label, one-level hierarchy), and listbox/select fields via a xoluman-owned xoluman_field_meta entity type (schema-less, same established pattern as T-09's blob folders -- no new xolu API needed for either feature). Horacio: implement the easy and medium items, document what's postponed. DONE, tested, verified only at the unit level (no e2e yet this pass): internal/fieldmeta (LoadForEntityType/ResolveOptions, static and ref-sourced options, 11 tests); formengine.RenderFields refactored from 4 positional args to a RenderOptions struct (Values/Errors/ReadOnly/FieldOptions/RefLinks) -- select rendering takes priority over type-based dispatch, required fields skip the empty leading choice; internal/ui/refs.go's resolveFormOptions wires both into every entity form call site (NewForm/Create/EditForm/Update), building ref links from EntitySchema.Refs (confirmed this already gives the target entity type directly -- no guessing needed) with a name/title/label heuristic for the link text, falling back to "type #id" (including when the fetch itself fails, so a broken reference stays visible and clickable rather than vanishing). NOT DONE, explicitly postponed: (1) ref links in the entity LIST preview -- cheap, no extra fetches needed since it only needs schema.Refs already loaded, just not wired into entityListTable yet; (2) the one-level side-panel hierarchy view Horacio asked for -- resolveFormOptions already computes everything it would need, just needs a rendering pass; (3) grid editor's Tabulator list-editor integration for select fields -- config shape already verified against the real vendored source (plain {"key":"label"} object, confirmed, not guessed) but buildGridColumns doesn't consume fieldmeta yet; (4) end-to-end verification against real xolu -- everything above is unit-tested only so far, not yet proven against a live xolu instance the way T-05/T-10/T-11's grid API were; (5) full nested inline EDITING of a linked document within the parent form -- assessed as High difficulty and explicitly recommended against for v1 (real complexity around nested form state and save semantics: does saving the parent also save the child, as separate requests, with what partial-failure behaviour) -- link-plus-preview gets most of the value for a fraction of the risk; not started, no plan to start without a separate design pass.

