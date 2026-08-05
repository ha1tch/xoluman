# xoluman — Resolution Record

Append-only, newest first. Closed items are moved here verbatim from
`TRACKING.md`, stamped with closing version and date, per the closure
procedure.

## [0.7.3] T-14 — FSM def module: full visual viewer + editor, fsm-toolkit validation, layout persistence (v0.7.3, 2026-08-05)

Theme: fsm-def-module · closed 0.7.3 · 2026-08-05


Prompted by Horacio noticing xoluman's module registry (T-08) only ported Seam's nav-registration piece, not the fuller pattern: Seam's actual modules.Module has MountRoutes func(*http.ServeMux) -- each module owns its own route registration, Registry.MountAll mounts everything in one call -- instead of every route being hand-wired centrally in internal/server/server.go the way connections.go and entities.go currently are. Extending modules.Module with MountRoutes and refactoring the existing two feature areas onto it, then building this new module the same way from the start, rather than adding a third inconsistent wiring style. Scope for the FSM def module itself, checked against the real xolu source rather than assumed: server-side has full CRUD (POST/GET/PUT/DELETE /api/v2/fsm/def, plus /fsm/def/validate) but pkg/client only wraps ListMachineDefs/GetMachineDef -- read-only, no create/replace/delete/validate methods exist to build a real editor against. Also checked MachineSpec/TransitionDef's actual wire shape (states carry a terminal flag; transitions carry guard/output/set, not just a label) against Seam's actual FSM canvas engine (seam-fsm-editor.js, genuinely Evan Wallace's code) -- confirmed the canvas only ever stores one free-text label per transition, no guard/output/set fields anywhere in it; the March-2026 fsm-toolkit feature request (docs in the Seam checkpoint) was for the data model only, the canvas UI was never extended to match. Reusing it as-is would silently collapse guard/output/set into one label field on save -- a real data-loss trap, not a cosmetic gap. Scoped down to what's honestly buildable now: a list + detailed read-only textual/tabular viewer (states with terminal flags, transitions with full from/input/to/guard/output/set, variables) -- represents the real data completely, no lossy conversion, and is genuinely useful on its own for understanding what an FSM actually does. The visual canvas (Wallace engine, read-only rendering or a properties-panel extension for real editing) and actual create/edit capability are separate, later work -- the latter blocked on client methods added to T-13.

**Update, 2026-08-04, xolu v0.26.0:** the blocker above is resolved. `CreateMachineDef`/`ReplaceMachineDef`/`DeleteMachineDef`/`ValidateMachineDef` now exist officially (`pkg/client/schema.go`), confirmed directly against the real v0.26.0 source, matching docs/xolu-requests-fsm-def.md closely. Bonus over what was asked: `analysis` (reachability, determinism, cycles, warnings) comes back as a typed `MachineDefAnalysis` struct on all four, not raw JSON, and `GetMachineDef` gained a `ParsedAnalysis()` method decoding into the same struct — worth rendering directly rather than parsing by hand. Two behavioral notes from the delivery worth keeping in mind when this is built: `ReplaceMachineDef` affects future machine creation only (no retroactive effect on already-running machines — surface this in the UI if editing a definition with live machines); `DeleteMachineDef` has no reference check at all (xolu doesn't expose "count machines by definition ID" — client-side cross-referencing needed for a "warn before deleting something in use" affordance, if wanted).

`internal/xoluext/fsmdef.go` — the hand-rolled raw-HTTP workaround for these same four operations, built before the official client had them — has been deleted (zero callers anywhere in the codebase, confirmed by grep before removing it; this was xoluman's own temporary code, not something the FSM-feature track had started depending on). Use the real `xolu/pkg/client` methods directly when this work resumes, not a reconstruction of the old workaround.

**Done, 2026-08-04, full viewer and editor built (superseding the earlier read-only-viewer scoping above and the out-of-scope note that followed it -- both explicitly asked for and delivered this session).** internal/ui/fsmdef.go: List, NewForm/View (the editor shell), GetData, Create, Update, Delete, ValidateLocal, SaveLayout -- full CRUD against the now-official client.CreateMachineDef/ReplaceMachineDef/DeleteMachineDef, using the real MachineSpec/TransitionDef wire shape directly (states with terminal flags, transitions with full from/input/to/guard/output/set) -- the earlier data-loss concern about Seam's canvas engine collapsing guard/output/set into one label does not apply here, since this editor was built from scratch with its own explicit fields for each, not a reuse of that engine. web/static/js/fsm-editor.js: a real visual canvas (SVG, draggable states, transition arrows, self-loops), not a plain form. Two specific asks folded in: (1) validation before submission, via github.com/ha1tch/fsm-toolkit -- converts MachineSpec into fsm-toolkit's own *fsm.FSM (states/initial/transitions map cleanly; guards, variable Set clauses, GC policy, input queries do not, and are called out explicitly as skipped-check notes rather than silently ignored) and runs its Validate()/Analyse(), debounced, live, with zero network calls to xolu -- complementary to xolu's own ValidateMachineDef, not a replacement for it. (2) visual-diagram persistence, following the same separation fsm-toolkit's own fsmedit tool uses (pkg/fsmfile.Layout -- position data kept structurally separate from the abstract machine, not the same zip+hex+TOML file format, which is built for local files on a TUI tool and isn't a fit for a web app talking to a remote xolu instance) -- a new xoluman_fsm_layout bookkeeping entity, same established pattern as xoluman_field_meta, keyed by the machine def's own ID. Three real bugs found and fixed only by actually running it (Playwright), none visible from reading the code: a panic on the list page from passing an uncalled mi.H function as a child instead of invoking it; transition from-fields double-JSON-encoded (JSON.stringify'd once too many before the outer JSON.stringify of the whole spec); and the entire SVG canvas rendering in the wrong DOM namespace because nested per-state/per-transition sub-templates used Lit's html tag instead of its dedicated svg tag -- confirmed via getBBox() (didn't exist) and namespaceURI (xhtml, not svg) before the fix, verified after via a real drag-and-persist round trip (dragged a state, re-fetched the layout from the server, confirmed the real new coordinates). Also fixed along the way: JSON API endpoints (Create/Update/Delete/GetData/ValidateLocal/SaveLayout) were returning writeUpstreamError's full HTML error page on failure -- correct for page handlers, broken for a JS component reading fetch() responses as JSON -- given a dedicated writeUpstreamErrorJSON path instead. Known, disclosed limitation: two transitions between the same state pair draw overlapping labels at the same midpoint (a curve-apart fix, not attempted this pass). 10 Go tests for the converter/validation logic; full CRUD (create/read/update/delete/validate/layout-save) verified end-to-end against a real xolu instance, not just unit-tested. per Horacio's instruction — the FSM feature set is being worked on separately. This update exists so whoever picks it up next sees the current, correct state rather than the stale T-13-blocked framing.

Cross-ref: CHANGELOG 0.7.3.

## [0.7.2] T-15 — REF field navigation + listbox/select fields via xoluman_field_meta — done, nested inline editing deliberately out of scope (v0.7.2, 2026-08-05)

Theme: ref-and-listbox · closed 0.7.2 · 2026-08-05


Two features from an explicit difficulty assessment (2026-08-03): REF fields as navigable links, and listbox/select fields via a xoluman-owned xoluman_field_meta entity type (schema-less, same established pattern as T-09's blob folders -- no new xolu API needed for either). Horacio: implement the easy and medium items, document what's postponed. All of the following now done and verified end-to-end (not just unit-tested) against the real CRM demo: internal/fieldmeta (LoadForEntityType/ResolveOptions/RememberRefTarget/LookupRememberedTarget); formengine.RenderFields takes a RenderOptions struct (Values/Errors/ReadOnly/FieldOptions/RefLinks/RefTargets); ref links wired into every entity form call site AND the entity list preview (entityListTable), building links from EntitySchema.Refs plus remembered targets, not schema-declared targets alone; a lightning-icon jump button (RefJumpButton) opens a ref link's target directly in a modal instead of a full navigation, delivering comparable value to the originally-requested side-panel hierarchy view via a different, simpler mechanism (modal reuse rather than a new panel layout) -- verified by actually clicking it, which surfaced and fixed two real modal.js bugs along the way (documented in modal.js's own comments); the grid editor's Tabulator list-editor now consumes fieldmeta (buildGridColumns takes fieldOptions), giving configured fields the same dropdown in the grid as the entity form -- verified end-to-end with a real field_meta entry, a real edit through the dropdown, a real save, and a direct API check confirming the value actually changed, in both light and dark mode. NOT DONE, deliberately: full nested inline EDITING of a linked document within the parent form -- assessed as High difficulty and explicitly recommended against for v1 (real complexity around nested form state and save semantics: does saving the parent also save the child, as separate requests, with what partial-failure behaviour) -- link-plus-jump gets most of the value for a fraction of the risk; not started, no plan to start without a separate design pass if ever requested. (2026-08-03): REF fields as navigable links (clickable, resolved-label, one-level hierarchy), and listbox/select fields via a xoluman-owned xoluman_field_meta entity type (schema-less, same established pattern as T-09's blob folders -- no new xolu API needed for either feature). Horacio: implement the easy and medium items, document what's postponed. DONE, tested, verified only at the unit level (no e2e yet this pass): internal/fieldmeta (LoadForEntityType/ResolveOptions, static and ref-sourced options, 11 tests); formengine.RenderFields refactored from 4 positional args to a RenderOptions struct (Values/Errors/ReadOnly/FieldOptions/RefLinks) -- select rendering takes priority over type-based dispatch, required fields skip the empty leading choice; internal/ui/refs.go's resolveFormOptions wires both into every entity form call site (NewForm/Create/EditForm/Update), building ref links from EntitySchema.Refs (confirmed this already gives the target entity type directly -- no guessing needed) with a name/title/label heuristic for the link text, falling back to "type #id" (including when the fetch itself fails, so a broken reference stays visible and clickable rather than vanishing). NOT DONE, explicitly postponed: (1) ref links in the entity LIST preview -- cheap, no extra fetches needed since it only needs schema.Refs already loaded, just not wired into entityListTable yet; (2) the one-level side-panel hierarchy view Horacio asked for -- resolveFormOptions already computes everything it would need, just needs a rendering pass; (3) grid editor's Tabulator list-editor integration for select fields -- config shape already verified against the real vendored source (plain {"key":"label"} object, confirmed, not guessed) but buildGridColumns doesn't consume fieldmeta yet; (4) end-to-end verification against real xolu -- everything above is unit-tested only so far, not yet proven against a live xolu instance the way T-05/T-10/T-11's grid API were; (5) full nested inline EDITING of a linked document within the parent form -- assessed as High difficulty and explicitly recommended against for v1 (real complexity around nested form state and save semantics: does saving the parent also save the child, as separate requests, with what partial-failure behaviour) -- link-plus-preview gets most of the value for a fraction of the risk; not started, no plan to start without a separate design pass.

Cross-ref: CHANGELOG 0.7.2.

## [0.7.1] T-21 — Client.Health() still doesn't apply auth — resolved via xolu v0.27.0's TestConnection() (v0.7.1, 2026-08-04)

Theme: xolu-client-ext · closed 0.7.1 · 2026-08-04


The only item from the original docs/xolu-requests.md not addressed by xolu v0.25.0 -- confirmed by direct re-inspection of pkg/client/client.go's Health() at the time, unchanged, still never setting an Authorization header. Re-filed as its own focused ask. RESOLVED: xolu v0.27.0 shipped Client.TestConnection() (hits GET /api/v1/schemas, genuinely authenticated), delivered as part of the broader response to docs/xolu-requests.md. xoluman's Test and TestUnsaved handlers (internal/ui/connections.go) switched from Health() to TestConnection() the same session -- confirmed directly in source, and covered by a test proving the actual point: a server that's reachable but rejects the credential now correctly reports failure, which Health() structurally could never detect. (confirmed by direct re-inspection of pkg/client/client.go's Health() -- unchanged, still never sets an Authorization header). xoluman's 'Test connection' and 'Test before saving' (v0.6.6) can therefore still only confirm the server is reachable, never that the configured credential is actually valid -- a connection with a wrong or expired token looks identical to a correctly-configured one. Re-file as its own focused ask to the xolu team rather than letting it stay buried as one item in a since-closed six-item document.

Cross-ref: CHANGELOG 0.7.1.

## [0.6.23] T-24 — xolu server bug: updating an entity was broken by two separate bugs — xolu's id/_version validation (fixed v0.27.0) and xoluman's own undeclared-ref-target gap (now fixed) (v0.6.23, 2026-08-04)

Theme: xolu-client-ext · closed 0.6.23 · 2026-08-04


Severe bug, reported as 'saving with POST is not working': updating an existing entity with a populated ref field always failed with XOLU-VL001. xoluman's own working theory at the time -- an undeclared ref target -- reproduced the symptom exactly via raw curl, but was WRONG on the cause: the xolu team checked it directly before trusting it, and a plain schema with no ref fields at all reproduced the identical failure, while declaring a target did not fix it. The real cause, xolu v0.27.0 (T-159): PUT, PATCH, and save all validated a document that already contained id (and for PATCH, _version) -- system fields no schema ever declares -- and additionalProperties:false correctly rejected them per its own spec, on every single update regardless of what was actually changed. POST (create) never hit this, since a not-yet-created entity has no id yet. Fixed server-side via stripSystemFieldsForValidation, confirmed directly against pkg/server/handlers.go and server.go's three call sites (PUT, PATCH, save), and verified empirically: a correctly-shaped direct PUT against the exact examples/crm repro now succeeds. A SEPARATE, genuinely distinct bug was found once that fix was verified: xoluman's own form was submitting a bare number for any ref field whose schema doesn't declare a target (confirmed: exactly what examples/crm's own seed script does for every ref field except users'), which xolu correctly rejects regardless of the id/_version fix -- affecting both create and update identically, and entirely xoluman's own gap, nothing to do with the xolu team's fix. Closed properly: formengine now renders a companion f.Name+__ref_entity input for exactly this case (RenderOptions.RefTargets, refInput, ParseFormValues all updated, 7 new tests), letting the person supply the target entity type directly. Verified end-to-end through xoluman's own actual web form against the real CRM demo -- both create and update now genuinely succeed, confirmed persisted in the real data, not just each piece in isolation.: a format:ref property with no explicit target ({"type":"object","format":"ref"}, no target key -- exactly what examples/crm's own seed script uses for every ref field except users' fields) can be CREATED fine, but any UPDATE (PUT or PATCH) to a row that already has a value in that field fails with XOLU-VL001 'id: unexpected field' -- confirmed this happens even when the PATCH body doesn't touch the ref field at all, meaning something on the update path re-validates the EXISTING stored value (likely its own embedded read-shape, which carries id plus other target-document fields) against the write-shape validator and rejects it. Isolated precisely via raw curl on the same instance: registering a second schema with target explicitly declared makes the identical write-shape PUT succeed on the same server. Practical effect: any xoluman connection to the CRM demo (or any real schema shaped the same way) can create and read every entity type correctly but can never update one again once a ref field has a real value -- exactly matching the reported 'saving with POST is not working,' for the specific case of updating an existing row. New-row creation is unaffected. Filed to xolu team; no client-side fix possible since raw, correctly-shaped requests reproduce this with nothing between the request and the server.

Cross-ref: CHANGELOG 0.6.23.

## [0.6.23] T-23 — xolu client bug: GetEntitySchema/DefineEntitySchema wrongly tenant-prefixed — fixed in xolu v0.26.2, xoluman workaround removed (v0.6.23, 2026-08-04)

Theme: xolu-client-ext · closed 0.6.23 · 2026-08-04


Real, severe bug: any xoluman connection with a tenant configured was completely broken for entity browsing -- every GetEntitySchema call (Show, EditForm, NewForm, Create, Update, GridView, ImportPreview) failed with XOLU-ST004 'Invalid ID'. Root cause confirmed precisely: Client.buildURL applied the tenant path prefix to every request once a tenant was set, with no per-endpoint awareness; /schema/{entity} was registered on the server only at the global level, never duplicated under the tenant router, unlike /entities, schema-suggestion, and both promote endpoints which genuinely were. Reproduced byte-for-byte via direct curl before touching any xoluman code, using xolu's own examples/crm demo (the realistic, tenant-scoped, multi-entity-type dataset that finally surfaced it after several sessions of not being able to reproduce with tenant-less test data). Filed to the xolu team (docs/xolu-requests-tenant-schema.md). Worked around in xoluman meanwhile: internal/xoluext.BuildSchemaClient(), a second tenant-less client used only for schema calls, across all 6 real call sites. RESOLVED PROPERLY, not just worked around: xolu v0.26.2 shipped buildURLRoot, teaching GetEntitySchema/DefineEntitySchema/the schema-list call to skip the tenant prefix on their own -- confirmed directly against pkg/client/client.go's own doc comment on the fix, which had shipped before this team's letter even arrived. xoluman's workaround was removed once this was verified: BuildSchemaClient and schemaClientFor deleted entirely, all 6 call sites reverted to the regular client, 2 obsolete tests replaced with one confirming the regular client now correctly handles a schema fetch even with a tenant configured. was completely broken for entity browsing — every GetEntitySchema call (Show, EditForm, NewForm, Create, Update, GridView, ImportPreview) failed with XOLU-ST004 'Invalid ID'. Root cause confirmed precisely: Client.buildURL applies the tenant path prefix to every request once a tenant is set, with no per-endpoint awareness; /schema/{entity} is registered on the server only at the global level (confirmed against pkg/server/server.go's own route table), never duplicated under the tenant router, unlike /entities, schema-suggestion, and both promote endpoints which genuinely are. A tenant-scoped client's schema fetch for 'companies' therefore requests /api/v1/tenant/{tenant}/schema/companies -- xolu's router matches this against the entity-by-id pattern instead, landing 'companies' in the numeric {id} slot, failing strconv.Atoi. Reproduced byte-for-byte via direct curl before touching any xoluman code, using xolu's own examples/crm demo (the realistic, tenant-scoped, multi-entity-type dataset that finally surfaced it after several sessions of not being able to reproduce with tenant-less test data). Fixed in xoluman: internal/xoluext.BuildSchemaClient() builds a second, tenant-less client per connection used only for the two schema calls; all 6 real call sites migrated (Show, resolveEntityFields covering NewForm/Create, EditForm, Update, GridView, ImportPreview); 2 call sites (GridView, ImportPreview) had their now-unnecessary regular clientFor call removed entirely rather than left unused. 7 new tests. Verified end-to-end against examples/crm: the exact previously-failing request now returns 200 with correct data; a full sweep across all 6 entity types' list pages, edit pages, and ref links (121 URLs) found zero failures. Request filed to the xolu team (docs/xolu-requests-tenant-schema.md) since the correct long-term fix is in the client library itself; xoluman's workaround is not something to keep maintaining once that lands.

Cross-ref: CHANGELOG 0.6.23.

## [0.6.22] T-25 — DXP transaction support in the query editor — proposal written, not started (v0.6.22, 2026-08-04)

Theme: query-editor · closed 0.6.22 · 2026-08-04


Considered directly: DxpDef (registered template, participants with $ref binding placeholders) and DxpTxn (DefID + Bindings, dispatches synchronously) confirmed against the real client (pkg/client/dxp.go, types_dxp.go). Structurally different from OQL/Sulpher/REST -- a def picker + dynamically-generated binding form, not a code editor -- so proposed as its own view rather than a fourth query-editor mode. Binding-name extraction (walking Params for nested $ref placeholders) and dynamic form generation from an optional BindingsSchema are both genuinely new problems this codebase hasn't solved before. Not started -- recorded so the decision to defer is explicit, not a silent drop.

Cross-ref: CHANGELOG 0.6.22.

## [0.6.13] T-11 — Bulk/grid data editing: vendored Tabulator (vanilla JS) wrapped in a Lit shell (v0.6.13, 2026-08-04)

**Process correction, not new work.** `internal/ui/grid.go`, `grid_test.go`, and `web/static/js/grid-editor.js` were actually built, tested (11 tests), and verified end-to-end against real xolu in an earlier session pass — but the closure procedure (this record, the register update, a version bump, a changelog entry) was never run at the time; the work landed silently inside a larger response without its own checkpoint. Caught while reviewing the register before continuing further work. Re-verified fresh here (grid page loads with real vendored assets, `grid-data` GET reflects real seeded data, `grid-data` POST genuinely `Patch`es — `_version` incremented, `qty` changed 1→99) before closing, not just trusting the earlier record.


Theme: grid-editor · closed 0.6.13 · 2026-08-04


Confirmed 2026-08-03: Glide Data Grid is React-only (peer dependency on React 16-19, no vanilla build — verified, not assumed) and was ruled out on that basis, matching the project's no-React rule. Tabulator (github.com/olifolkerd/tabulator, MIT licensed) is the alternative — genuinely vanilla JS, ships as a plain JS+CSS pair (dist/js/tabulator.min.js + dist/css/tabulator.min.css), vendorable exactly like htmx and Lit already are. Wrapped in a Lit shell per Horacio's suggestion, this reuses the same architecture already established for the FSM editor and planned for the graph editor: a Lit shell (toolbar/theming/lifecycle) around a swappable, framework-agnostic engine. All three of xoluman's richer embedded widgets (FSM editor, graph editor, grid editor) end up sharing one consistent pattern rather than three different ones. Distinct from T-07: T-07 is a single-entity schema-driven form; this is bulk editing across many rows/cells of one entity type at once (paste, keyboard nav, multi-cell selection) — a materially bigger feature, correctly sequenced after T-07 ships.

**Design pass, 2026-08-03:**

- **Additive, not a replacement.** The grid is a second view of an entity type, reached from a "Grid view" link on the existing paginated list (T-10) — not a replacement for it. The simple list+form flow stays the default for occasional single-row edits; the grid is specifically for editing many rows quickly (paste, fill-down, keyboard nav).
- **Scope: bulk edit only, not bulk create/delete.** Creating and deleting rows already have homes — the entity form (T-07) and import (T-05) for creation, the existing delete-confirm flow for removal. The grid's job is editing values across many existing rows at once. Keeping create/delete out of it avoids duplicating either.
- **Real gotcha caught at design time, not as a data-loss bug later: writes must be PATCH, never PUT.** The grid can't reasonably show every field of a wide schema as a spreadsheet column — some columns will be omitted for readability, the same way T-10's list preview caps at 4. If a save sent `Client.Update` (a full-document PUT), any field not shown as a grid column would be silently dropped from the document on save. `Client.Patch` (partial update, PATCH) is the only correct choice here — it only touches the fields actually present in what's sent, exactly matching what a partial-column grid needs. This must not be revisited casually later; it's the one design fact that would turn "edit conveniently" into "silently delete data."
- **Column set:** every non-object/array scalar field from the schema (unlike the list preview's 4-column cap, which exists for glanceability, not editability — a grid meant for bulk editing should show what there is to edit). Object/array fields excluded, same reasoning as everywhere else in xoluman: not suited to a spreadsheet cell, formengine's own edit form is where those live. Ref fields editable by raw target ID, same v1 limitation as elsewhere — no lookup-by-name picker yet.
- **Data transfer, client ↔ server:** a dedicated JSON endpoint per entity type (`GET .../grid-data`) that Tabulator's own remote-pagination mode (`ajaxURL` + `pagination: "remote"`) consumes directly — reusing `Client.List`'s existing pagination rather than loading an entire (potentially large) entity type into the browser at once. Saves batch through a matching `POST .../grid-data` accepting per-row partial changes, executing one `Patch` per row, independently — same "one row's failure doesn't affect another's" philosophy already established for import (T-05), reported back the same way.
- **Real limitation, stated plainly:** the Lit+Tabulator component itself is client-side interactive JavaScript — this sandbox has no real browser to click into, drag-select cells in, or paste into. What gets verified here is everything that can be: the JSON API endpoints against real xolu (fully Go-testable, same rigor as everything else), asset vendoring and serving, and that the page assembles and the script loads without error. Actual interactive grid behaviour — editing, keyboard nav, the save flow's real UX — needs Horacio's own hands, the same category of gap as T-04's live-keyring guard.

Cross-ref: CHANGELOG 0.6.13.

## [0.6.12] T-12 — Query editor: CodeMirror 6 for all three query modes (OQL/Sulpher/REST), Lit shell (v0.6.12, 2026-08-04)

Theme: query-editor · closed 0.6.12 · 2026-08-04


Confirmed 2026-08-03 with Horacio's precise language facts: OQL is a subset of T-SQL, Sulpher is almost exactly openCypher9. Shiki was the original candidate but is superseded — it's a highlighter only (codeToHtml, no cursor/typing/selection), not an editor, and requires vendoring the Oniguruma WASM binary alongside it. CodeMirror 6 (MIT, github.com/codemirror, genuinely vanilla JS core — 'import {EditorView, basicSetup} from "codemirror"', no framework needed) is the actual editable component, and purpose-built official language packages cover both DSLs better than Shiki's generic/community grammars would have: OQL uses @codemirror/lang-sql's built-in MSSQL dialect (confirmed via its changelog's explicit MSSQL keyword/builtin coverage, and docs on MSSQL-style bracket-quoted identifiers) -- safe to use the T-SQL dialect for a T-SQL subset, since anything OQL uses is valid T-SQL by construction, zero custom grammar work needed. Sulpher uses @neo4j-cypher/codemirror (Apache-2.0, framework-agnostic base package -- the React wrapper @neo4j-cypher/react-codemirror is separate and not needed), built on Neo4j's real ANTLR4 Cypher grammar plus semantic analysis rather than a regex-based TextMate approximation, and it ships autocompletion, linting, and formatting already, not just highlighting. REST query bodies use @codemirror/lang-json (official, MIT). Net effect: Shiki and its WASM dependency are dropped entirely -- CodeMirror alone, three official per-language packages, better accuracy on both real DSLs than the generic grammars Shiki would have supplied. Wrapped in a Lit shell, this is the fourth instance of the same architecture already used for the FSM editor, planned for the graph editor, and T-11's grid editor: a Lit shell (toolbar/theming/lifecycle) around a swappable, framework-agnostic engine -- one consistent pattern across every rich embedded widget in xoluman rather than four different ones.

Cross-ref: CHANGELOG 0.6.12.

## [0.6.10] T-09 — Blob browser: virtual hierarchy over the flat key store (v0.6.10, 2026-08-04)

Theme: blob-browser · closed 0.6.10 · 2026-08-04


**Note, 2026-08-04:** the "After T-01" blocker this item originally
carried is stale — T-01 was discarded (see its RESOLVED.md correction
note) and its actual ask was folded into T-13, now closed by the xolu
team's v0.25.0 delivery. T-18 was filed as a near-duplicate restating
this same unblocking; closed as merged back in here rather than left
as a second open item for the same work. One more thing worth knowing,
reconfirmed against v0.25.0's validateBlobKey while checking the
delivery: leading `.` and the literal keys `.`/`..` are also reserved,
not just `/` and `\`.

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

Cross-ref: CHANGELOG 0.6.10.

## [0.6.9] T-20 — Schema promotion UI — new entity-browser feature (preview + PromoteFlex/PromoteStrict) (v0.6.9, 2026-08-04)

Theme: entity-browser · closed 0.6.9 · 2026-08-04


Not requested by Horacio originally -- xolu team built it as a bonus alongside ListEntities, same v0.25.0 release. GetSchemaSuggestion(ctx, type) previews a heuristic-inferred schema with per-field confidence/reasoning, no side effects. PromoteFlex(ctx, type, schema) is fast/synchronous but does not migrate pre-existing rows into the new adapted table (check result.Warning). PromoteStrict(ctx, type, schema) validates every existing row first and only migrates if all pass, atomically -- rejection is a normal PromoteJobStatus (PromoteJobRejected), not a Go error, with job.Failures naming exactly which rows and why. Pass nil schema to either Promote method to auto-infer rather than using a suggestion. Natural UI: on the entity type list (once it shows ListEntities' has_schema flag), a schemaless row gets a 'Promote to schema' action -> preview via GetSchemaSuggestion, editable, then PromoteStrict by default (PromoteFlex as an explicit opt-in with the data-loss warning surfaced, not the default).

Cross-ref: CHANGELOG 0.6.9.

## [0.6.8] T-16 — Schema-less entity editing: EditForm/Update/NewForm/Create still assume a registered schema (v0.6.8, 2026-08-04)

Theme: entity-browser · closed 0.6.8 · 2026-08-04


Follow-on from the Show/List fix (v0.6.5, CHANGELOG) for 'schemas are not obligatory in xolu' -- that fix covers browsing (Show infers preview columns from fetched data when no schema exists; List gained a jump-by-name bypass for discovery). EditForm/Update/NewForm/Create were not touched and still call GetEntitySchema as a hard prerequisite, so a schema-less entity type can be viewed in the list but not edited or created through the generic form yet. Real fix for EditForm/Update: when GetEntitySchema 404s but the target document itself loads fine, derive the field list from that document's own keys/JSON value types (the same inferFieldsFromEntities approach Show now uses) rather than failing. NewForm/Create is harder -- a brand new document has nothing to infer from if the type has zero existing rows; reasonable v1 answer is a clear message rather than a silent failure when there's truly nothing to infer from, falling back to inferring from an existing document when at least one exists.

Cross-ref: CHANGELOG 0.6.8.

## [0.6.7] T-03 — Add a minimal Raw request method to `xolu/pkg/client` (v0.6.7, 2026-08-04)

**Resolved by the xolu team, not by xoluman code.** `Client.Raw` shipped in xolu v0.25.0, confirmed directly against `pkg/client/raw.go` — matches this request closely, no reinterpretation. Closing the ask; the actual REST-console feature that needs it is separate, still open, tracked under T-12.


Theme: xolu-client-ext · closed 0.6.7 · 2026-08-04


`Client.do`/`doURL` are unexported. The REST console needs to issue
arbitrary method+path+body requests using the connection's already-
configured auth, so a small public `Raw(ctx, method, path, body)
(status int, body []byte, err error)` on the client would cover it —
same reasoning as T-01/T-02, keep auth/retry logic in one place.

**Reframed 2026-08-03:** not xoluman-implementable, same correction as
T-02 — this is a request to the xolu team (T-13,
`docs/xolu-requests.md`), not something built directly into Horacio's
local checkout.

Cross-ref: CHANGELOG 0.6.7.

## [0.6.7] T-02 — Add Export method to `xolu/pkg/client` (v0.6.7, 2026-08-04)

**Resolved by the xolu team, not by xoluman code.** `Client.Export(ctx, w)` shipped in xolu v0.25.0 — redesigned from the original synchronous-stream ask into an async, tenant-scoped, blob-backed mechanism (the old `GET /api/v1/export` had zero tenant scoping — a real security problem, not a style choice), but the caller-facing experience matches what was originally requested: one call, hides the polling. Closing the ask; the actual backup/export UI feature that needs it is separate, still open, tracked under T-22.


Theme: xolu-client-ext · closed 0.6.7 · 2026-08-04


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

Cross-ref: CHANGELOG 0.6.7.

## [0.6.7] T-19 — REST console (part of T-12) — now genuinely unblocked by xolu v0.25.0's Raw method (v0.6.7, 2026-08-04)

**Closed as a duplicate, not as completed work.** This restated an unblocking already captured in T-12 itself (updated in the same pass). No REST console code shipped — that work is still open, tracked under T-12.


Theme: query-editor · closed 0.6.7 · 2026-08-04


T-12's REST mode needed a generic authenticated-request capability the official client didn't expose (do/doURL/doOnce are unexported) -- that was the whole reason T-03 was filed. xolu v0.25.0 shipped Client.Raw(ctx, method, path, contentType, body) -- confirmed directly against pkg/client/raw.go: no tenant-prefixing (caller controls the exact path), no structured-error decoding (caller inspects StatusCode directly), single-attempt. Exactly the shape needed. Not started this session. Note for whoever builds this: xoluext/fsmdef.go's hand-rolled raw-HTTP helpers (built before Raw existed, for the FSM def write methods that still aren't in the official client) could be refactored to call c.Raw() internally instead of duplicating auth-header/URL-building logic, while keeping fsmdef.go's own FSM-specific structured-error decoding on top -- worth doing as a cleanup, not urgent.

Cross-ref: CHANGELOG 0.6.7.

## [0.6.7] T-18 — Blob browser (T-09) — now genuinely unblocked by xolu v0.25.0's Blob client methods (v0.6.7, 2026-08-04)

**Closed as a duplicate, not as completed work.** This restated an unblocking already captured in T-09 itself (updated in the same pass). No blob browser code shipped — that work is still open, tracked under T-09.


Theme: blob-browser · closed 0.6.7 · 2026-08-04


Original T-09 design (virtual hierarchy over the flat key store, colon as in-key delimiter since / and \\ are xolu-reserved -- reconfirmed against v0.25.0's validateBlobKey, also reserves leading '.' and the literal '.'/'..' keys, worth knowing) is unblocked now that BlobPut/BlobGet/BlobHead/BlobDelete/BlobList/BlobUsage exist in the real client. Not started this session -- filed so it does not silently fall off the list now that its blocker is gone.

Cross-ref: CHANGELOG 0.6.7.

## [0.6.7] T-17 — Replace T-16 jump-by-name workaround with real ListEntities discovery (v0.6.7, 2026-08-04)

Theme: entity-browser · closed 0.6.7 · 2026-08-04


xolu v0.25.0 shipped Client.ListEntities(ctx, includeGraph) -- lists every entity type with actual data, schemaless or not, with row counts/schema status/adapted-table info. This is the real, proper fix for what T-16's jump-by-name form patched around. Replace entities.List's discovery: show every entry from ListEntities (not just ListEntityTypes' schema-only view), with count and a has-schema indicator per row; keep the jump-by-name form as a fallback for typing an exact name directly rather than removing it outright, since it's still marginally faster for a known name. This also makes Show's own per-request schema-fetch-then-infer-on-404 fallback (T-16's fix) no longer strictly necessary for DISCOVERY, though it's still correct defense for actually viewing a schemaless type's rows -- keep that part as is.

Cross-ref: CHANGELOG 0.6.7.

## [0.6.7] T-13 — External request filed with the xolu team: client-library gaps, Health() auth gap, schema registration (v0.6.7, 2026-08-04)

Theme: xolu-client-ext · closed pending · 2026-08-04


Filed 2026-08-03 as docs/xolu-requests.md, a plain-language request document, not code. Corrects the mistake recorded in T-01's RESOLVED.md entry: xolu/pkg/client's blob and export methods were written directly into Horacio's local xolu checkout without asking, then discarded at his instruction once caught. xoluman does not modify xolu directly regardless of the go.mod replace directive making a local copy buildable -- changes to xolu are requests to the xolu team. See docs/KNOWN_ISSUES.md's recorded decision. The request document covers, reviewed comprehensively rather than just the blob/export items already in flight: Blob primitive client methods (blocks T-09 and the originally-scoped blob browser feature), a streaming Export method (blocks the backup feature; also flags EXPORT_API.md's stale manifest-shape documentation), a minimal Raw request method (blocks T-12's REST query console), a client wrapper for the existing but unwrapped POST /api/v1/schema/{entity} registration endpoint (needed for T-09's xoluman_blob_folder entity to get proper validation and to browse/edit correctly through xoluman's own generic data editor), and a functional correctness finding: Client.Health() never applies the configured auth header, confirmed by direct source inspection -- meaning xoluman's already-shipped 'Test connection' feature can only confirm the server is reachable, not that the stored token is actually valid. Closes when the xolu team responds; T-02/T-03/T-09 pick back up from whatever they decide.

**Second document filed 2026-08-03:** docs/xolu-requests-fsm-def.md -- a separate, focused request for the FSM definition write methods T-14 needs (CreateMachineDef/ReplaceMachineDef/DeleteMachineDef/ValidateMachineDef), split out from the main request rather than appended, since the xolu team was already mid-refinement on the /blob API and a tight, precisely-scoped second ask fit that timing better than folding into the larger document. Exact request/response shapes verified directly against pkg/server/v2_fsm_def_handlers.go, not inferred from docs.

**Response received 2026-08-04, xolu v0.25.0 (docs/xoluman-letter-2026-08-04.md).** Covers only the FIRST document (docs/xolu-requests.md) -- the FSM-def document is untouched, no response yet, tracked separately below since it's now genuinely distinct open work, not part of this closure. Every claim in the letter was checked directly against the v0.25.0 source before trusting it, not taken at face value -- all held up exactly as described.

Resolved: item 1 (Blob methods: BlobPut/BlobGet/BlobHead/BlobDelete/BlobList/BlobUsage, pkg/client/blob.go, client-side key validation confirmed) -- item 2 (Export, redesigned as async/tenant-scoped/blob-backed rather than the originally-scoped synchronous stream, for real security reasons: the old GET /api/v1/export had zero tenant scoping, one valid credential could pull the entire cross-tenant database; Client.Export(ctx, w) hides the polling, matches the original synchronous experience) -- item 3 (Raw method, pkg/client/raw.go, confirmed: no tenant-prefixing, no structured-error decoding, single-attempt -- exactly as requested) -- item 4 (DefineEntitySchema, pkg/client/schema.go).

NOT resolved, not mentioned in the letter at all: item 5 (Client.Health() still doesn't apply auth -- confirmed unchanged by direct re-inspection of the v0.25.0 source; "Test connection"/"Test before saving" still can only confirm reachability, not credential validity) and item 6 (the FieldDef.Type doc/code inconsistency, low-priority, never followed up on either side).

Two bonus items neither requested nor expected, both verified directly against source: ListEntities(ctx, includeGraph) -- lists every entity type with actual data, schemaless or not, with row counts/schema status/adapted-table columns/graph footprint; this is the real fix for the schema-less-discovery gap T-16 patched with a workaround (jump-by-name), and should replace that workaround, not just sit alongside it. And schema promotion (GetSchemaSuggestion/PromoteFlex/PromoteStrict) -- preview-then-promote a schemaless entity type to schemaful, PromoteStrict validating every existing row atomically before migrating anything (rejection is a normal PromoteJobStatus outcome, not a Go error).

Closing T-13 for the resolved items. Filing new tracking for: replacing T-16's workaround with real ListEntities-based discovery; the blob browser (T-09) now genuinely unblocked; the REST console (T-12) now genuinely unblocked via Raw; schema promotion as a new entity-browser feature; Health()'s auth gap re-filed as its own standalone item since it's now the only thing left unaddressed from the original ask.

Cross-ref: CHANGELOG pending.

## [0.6.1] T-08 — Module registry (registration + nav), no RBAC layer (v0.6.1, 2026-08-03)

Theme: ui-shell · closed 0.6.1 · 2026-08-03


Reversed from an earlier flat-nav assumption (2026-08-03) once graph
editor and FSM editor modules were confirmed as planned — three or more
distinct feature areas is exactly what Seam's module registry pattern
is for. Bring over registration + nav-building from Seam's
`internal/modules`, but not the role-based `VisibleTo(role)` gating:
xoluman has no multi-user/role model today. Shape the registry so a
visibility layer could be added later without a rewrite, without
building the RBAC itself now.

Also noted for when either editor module is picked up: Seam's own FSM
editor is not pure minty/htmx — it loads a Lit web-component shell
client-side (`LitImportMap` in Seam's `internal/ui/layout.go`). A graph
editor and an FSM editor are both node/edge visual tools and will
likely need the same treatment: server-rendered chrome around a
client-side canvas/SVG component, not a form. Not a decision to make
now, just recorded so it isn't a surprise later.

**Confirmed 2026-08-03: reuse the Lit shell for both editors.** Checked
what's actually underneath Seam's shell before recording this as
settled: `web/static/js/fsm-editor.js` is a thin Lit toolbar/theming
wrapper; the drawing engine it wraps, `seam-fsm-editor.js`, is Evan
Wallace's MIT-licensed Finite State Machine Designer — manually-placed
nodes and links, no auto-layout, no virtualization. That's a strong fit
for FSMs (a handful of hand-arranged states) and an open question for
the graph editor: Sulpher results can be arbitrarily large, and this
engine has nothing built in for large-graph layout. Seam vendors no
other graph-visualization library (`web/static/vendor/` is
htmx/lit/leaflet/autocomplete only — no d3/cytoscape/vis-network).
The Lit-shell *pattern* (toolbar/theming/canvas-lifecycle wrapper
around a swappable engine) reuses cleanly either way; whether the
*engine* itself reuses depends on whether the graph editor stays scoped
to small curated subgraphs (fits as-is) or needs to render arbitrarily
large query results (needs a different or additional engine). Decide
when the graph editor module is actually scoped, not now.

Cross-ref: CHANGELOG 0.6.1.

## [0.6.0] T-05 — Design entity import path (v0.6.0, 2026-08-03)

Theme: import-export · closed 0.6.0 · 2026-08-03


`EXPORT_API.md` confirms xolu has no import endpoint — restore is
stop-server-and-replace-the-SQLite-file, or re-create entities one at a
time. xoluman's "import" feature (as distinct from "backup", which is
the whole-DB export) therefore means: parse an input file (CSV/JSON),
map rows/records to entity fields against the target's schema, and issue
`Create` or `Commit` calls. Needs a design pass on error handling
(partial import failure, dry-run/preview) before implementation.

Cross-ref: CHANGELOG 0.6.0.

## [0.5.0] T-10 — Minimal server + UI shell (v0.5.0, 2026-08-03)

Theme: ui-shell · closed 0.5.0 · 2026-08-03


Discovered 2026-08-03 answering "can this be used to edit anything
yet?" — no. `connstore` (T-06) is a tested library with nothing calling
it: no `cmd/xoluman` entrypoint, no HTTP server, no UI at all. T-07 (the
schema-driven form renderer) is a pure rendering function and can be
unit-tested the same way `connstore` was, without a server — but even
once T-07 exists, nothing is actually usable until this ships too:
`cmd/xoluman/main.go`, `internal/server` (routing), and the minimum
`internal/ui` pages — connection list/add (using `connstore` directly),
entity type list, entity list, and the edit page wrapping T-07. This is
the milestone where a person can actually open a browser and do
something, as distinct from any single package being individually
correct.

**Shipped 2026-08-03 — connection management half:** `cmd/xoluman`
(binary entrypoint, `-addr` flag, builds the configured backend via
`config.Load()`), `internal/server` (stdlib `net/http.ServeMux`,
method+pattern routing — no router dependency, matching Seam's own
choice), `internal/ui` (minty-based page shell + nav, connection
list/add/delete pages), `internal/xoluext.BuildClient` (stored
`Connection` → `*xolu/pkg/client.Client`, auth mode and tenant mapped
correctly — verified against actual wire requests, not inferred), and a
working htmx "Test connection" button (`Client.Health` against the real
instance). Verified end-to-end against a real running xolu binary, not
just the test suite: create a connection pointing at it, Test reports
reachable, killing xolu flips it to unreachable with the real transport
error surfaced, delete removes it. 100%/90.2%/100% coverage
(server/ui/xoluext).

**Outstanding — the entity browser half:** entity type list, entity
list, and the edit page. Blocked on T-07 (the form renderer itself
doesn't exist yet) plus the list/browse pages around it, neither of
which is started.

**Correction 2026-08-03 — listing/modal conventions weren't actually
carried over.** Earlier in this project we agreed to keep Seam's modal
and listing invariants, adapted, not dropped. What the connection
management pages above actually shipped with instead: a full-page
navigation for "New connection" instead of a modal, a raw browser
`confirm()` for delete instead of a modal confirmation, and one-off
bespoke table markup instead of a reusable listing component — none of
which is what was agreed. Caught when asked directly to audit it, not
by self-review, which is the part worth remembering.

Fixed: `internal/ui/listing.go` (`ListPage`, `Table`,
`ModalTriggerButton`, `DeleteConfirmBody`, `WriteModalAware`) is a
trimmed adaptation of Seam's `listing.go` — same interaction pattern
(modal-loaded create/confirm via `hx-get` + `hx-target="#modal-body"`,
htmx-vs-direct-navigation branching so a bookmarked URL still gets a
full page), deliberately without Tailwind or Material Icons, which
Seam's version depends on and which "drastically trimmed down" was
never meant to include. `web/static/js/modal.js` is a from-scratch,
much smaller `XModal` controller covering the same API shape as Seam's
`SeamModal` (`open`/`close`/`setTitle`, backdrop-click and Escape to
dismiss) with the parts xoluman doesn't need cut: 4K-adaptive sizing,
dark-mode detection, fixed-footer extraction, inline-script
re-execution. Embedded into the binary via `web/embed.go` +
`go:embed`, matching Seam's own `web` package pattern. Connection
management refactored onto these; 93.3%/94.0% coverage
(server/ui), re-verified end-to-end against the real binary (modal.js
served, full-page vs. fragment branching on `HX-Request`, the full
create→delete-confirm→delete cycle).

Self-check note: this is a pattern worth watching for going forward,
not a one-off — stating an architectural decision in conversation and
then not verifying the actual code matches it once written. The
connections page passed its own tests and a real e2e run while still
diverging from what was agreed, because the tests were written against
what I'd built, not against what was decided.

Cross-ref: CHANGELOG 0.5.0.

## [0.4.0] T-01 — Add Blob primitive methods to `xolu/pkg/client` (v0.4.0, 2026-08-03)

Theme: xolu-client-ext · closed 0.4.0 · 2026-08-03


`docs/BLOB_API.md` documents `POST/GET/HEAD/DELETE /api/v1/blob{,/key}`,
`GET /api/v1/blob` (list), `GET /api/v1/blob/usage` — none are wrapped on
the Go client today. Add typed methods mirroring the existing CRUD method
style (`BlobPut`, `BlobGet`, `BlobHead`, `BlobDelete`, `BlobList`,
`BlobUsage`), tenant-scoped variant included. This is upstream work on
xolu itself, not xoluman-local, so other consumers benefit.

Cross-ref: CHANGELOG 0.4.0.

**CORRECTION, 2026-08-03, same day:** this closure was wrong. The
methods were written directly into Horacio's local xolu checkout without
asking — xoluman does not modify xolu directly; changes to xolu are
requests to the xolu team, not something this project implements
unilaterally, regardless of how the checkout is made locally available
(the `go.mod` `replace` directive exists for building against a local
copy, not for editing it). The code was discarded at Horacio's
instruction. History here stays as written per the append-only
convention; the actual current disposition of this need is tracked
under T-13 (`docs/xolu-requests.md`, a written request to the xolu team,
not an implementation). See `docs/KNOWN_ISSUES.md`'s recorded decision
on this boundary.

## [0.3.0] T-07 — Schema-driven generic form renderer (v0.3.0, 2026-08-03)

Theme: data-editor · closed 0.3.0 · 2026-08-03


Walks `client.EntitySchema.Fields` (draft-07 JSON Schema, extracted
client-side already) and renders flat input rows by JSON-Schema type.
Deliberately not Seam's `formengine` (no tabs, no `x-seam-relation`,
no visibility rules) — xoluman's editor is generic across arbitrary
entity types on arbitrary instances, not one product's asset model.

Cross-ref: CHANGELOG 0.3.0.

## [0.1.0] T-06 — `ConnectionStore` file backend + setup-time backend selection (v0.1.0, 2026-08-03)

Theme: connstore · closed 0.1.0 · 2026-08-03


Interface plus the plaintext-JSON backend (git-ignored, `0600`
permissions). Backend choice (`file` vs `keyring`) is a single
xoluman-level setting made once at first run, not per-connection —
confirmed with Horacio 2026-08-03.

Cross-ref: CHANGELOG 0.1.0.

