Version: 0.6.17
Last reviewed: 2026-08-03

# xoluman — Live Register

Open, actionable items only. Closed items move to `RESOLVED.md` in full,
per the closure procedure. See `docs/KNOWN_ISSUES.md` for intentional
limits and recorded decisions rather than open work.

## Status table

| ID | Summary | Theme | Priority | Status | Blocks/after |
|----|---------|-------|----------|--------|---------------|
| T-04 | Implement `ConnectionStore` keyring backend | connstore | P1 | ◐ | After: T-06 (closed, v0.1.0) |
| T-14 | FSM def module: list + detailed read-only viewer, real Module architecture | fsm-def-module | P2 | ☐ | After: none, self-contained |
| T-15 | REF field navigation + listbox/select fields via xoluman_field_meta | ref-and-listbox | P2 | ◐ | After: none, self-contained. No new xolu API needed for anything in scope. |
| T-21 | Client.Health() still doesn't apply auth — re-filed, only remaining item from the original xolu request | xolu-client-ext | P2 | ☐ | After: none, standalone |
| T-22 | Backup/export UI feature using xolu v0.25.0's Client.Export | import-export | P2 | ☐ | After: none, xolu v0.25.0 Client.Export available now |
| T-23 | xolu client bug: GetEntitySchema/DefineEntitySchema wrongly tenant-prefixed — workaround shipped, request filed | xolu-client-ext | P1 | ◐ | After: none. Waiting on xolu team response to docs/xolu-requests-tenant-schema.md. |

## Detail

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

## fsm-def-module

### T-14. FSM def module: list + detailed read-only viewer, real Module architecture

Theme: fsm-def-module · Priority: P2 · Status: ☐ · Blocks/after: After: none, self-contained

Prompted by Horacio noticing xoluman's module registry (T-08) only ported Seam's nav-registration piece, not the fuller pattern: Seam's actual modules.Module has MountRoutes func(*http.ServeMux) -- each module owns its own route registration, Registry.MountAll mounts everything in one call -- instead of every route being hand-wired centrally in internal/server/server.go the way connections.go and entities.go currently are. Extending modules.Module with MountRoutes and refactoring the existing two feature areas onto it, then building this new module the same way from the start, rather than adding a third inconsistent wiring style. Scope for the FSM def module itself, checked against the real xolu source rather than assumed: server-side has full CRUD (POST/GET/PUT/DELETE /api/v2/fsm/def, plus /fsm/def/validate) but pkg/client only wraps ListMachineDefs/GetMachineDef -- read-only, no create/replace/delete/validate methods exist to build a real editor against. Also checked MachineSpec/TransitionDef's actual wire shape (states carry a terminal flag; transitions carry guard/output/set, not just a label) against Seam's actual FSM canvas engine (seam-fsm-editor.js, genuinely Evan Wallace's code) -- confirmed the canvas only ever stores one free-text label per transition, no guard/output/set fields anywhere in it; the March-2026 fsm-toolkit feature request (docs in the Seam checkpoint) was for the data model only, the canvas UI was never extended to match. Reusing it as-is would silently collapse guard/output/set into one label field on save -- a real data-loss trap, not a cosmetic gap. Scoped down to what's honestly buildable now: a list + detailed read-only textual/tabular viewer (states with terminal flags, transitions with full from/input/to/guard/output/set, variables) -- represents the real data completely, no lossy conversion, and is genuinely useful on its own for understanding what an FSM actually does. The visual canvas (Wallace engine, read-only rendering or a properties-panel extension for real editing) and actual create/edit capability are separate, later work -- the latter blocked on client methods added to T-13.

**Update, 2026-08-04, xolu v0.26.0:** the blocker above is resolved. `CreateMachineDef`/`ReplaceMachineDef`/`DeleteMachineDef`/`ValidateMachineDef` now exist officially (`pkg/client/schema.go`), confirmed directly against the real v0.26.0 source, matching docs/xolu-requests-fsm-def.md closely. Bonus over what was asked: `analysis` (reachability, determinism, cycles, warnings) comes back as a typed `MachineDefAnalysis` struct on all four, not raw JSON, and `GetMachineDef` gained a `ParsedAnalysis()` method decoding into the same struct — worth rendering directly rather than parsing by hand. Two behavioral notes from the delivery worth keeping in mind when this is built: `ReplaceMachineDef` affects future machine creation only (no retroactive effect on already-running machines — surface this in the UI if editing a definition with live machines); `DeleteMachineDef` has no reference check at all (xolu doesn't expose "count machines by definition ID" — client-side cross-referencing needed for a "warn before deleting something in use" affordance, if wanted).

`internal/xoluext/fsmdef.go` — the hand-rolled raw-HTTP workaround for these same four operations, built before the official client had them — has been deleted (zero callers anywhere in the codebase, confirmed by grep before removing it; this was xoluman's own temporary code, not something the FSM-feature track had started depending on). Use the real `xolu/pkg/client` methods directly when this work resumes, not a reconstruction of the old workaround.

This item remains explicitly out of scope for the general xoluman work queue per Horacio's instruction — the FSM feature set is being worked on separately. This update exists so whoever picks it up next sees the current, correct state rather than the stale T-13-blocked framing.

## ref-and-listbox

### T-15. REF field navigation + listbox/select fields via xoluman_field_meta

Theme: ref-and-listbox · Priority: P2 · Status: ◐ · Blocks/after: After: none, self-contained. No new xolu API needed for anything in scope.

Two features from an explicit difficulty assessment (2026-08-03): REF fields as navigable links (clickable, resolved-label, one-level hierarchy), and listbox/select fields via a xoluman-owned xoluman_field_meta entity type (schema-less, same established pattern as T-09's blob folders -- no new xolu API needed for either feature). Horacio: implement the easy and medium items, document what's postponed. DONE, tested, verified only at the unit level (no e2e yet this pass): internal/fieldmeta (LoadForEntityType/ResolveOptions, static and ref-sourced options, 11 tests); formengine.RenderFields refactored from 4 positional args to a RenderOptions struct (Values/Errors/ReadOnly/FieldOptions/RefLinks) -- select rendering takes priority over type-based dispatch, required fields skip the empty leading choice; internal/ui/refs.go's resolveFormOptions wires both into every entity form call site (NewForm/Create/EditForm/Update), building ref links from EntitySchema.Refs (confirmed this already gives the target entity type directly -- no guessing needed) with a name/title/label heuristic for the link text, falling back to "type #id" (including when the fetch itself fails, so a broken reference stays visible and clickable rather than vanishing). NOT DONE, explicitly postponed: (1) ref links in the entity LIST preview -- cheap, no extra fetches needed since it only needs schema.Refs already loaded, just not wired into entityListTable yet; (2) the one-level side-panel hierarchy view Horacio asked for -- resolveFormOptions already computes everything it would need, just needs a rendering pass; (3) grid editor's Tabulator list-editor integration for select fields -- config shape already verified against the real vendored source (plain {"key":"label"} object, confirmed, not guessed) but buildGridColumns doesn't consume fieldmeta yet; (4) end-to-end verification against real xolu -- everything above is unit-tested only so far, not yet proven against a live xolu instance the way T-05/T-10/T-11's grid API were; (5) full nested inline EDITING of a linked document within the parent form -- assessed as High difficulty and explicitly recommended against for v1 (real complexity around nested form state and save semantics: does saving the parent also save the child, as separate requests, with what partial-failure behaviour) -- link-plus-preview gets most of the value for a fraction of the risk; not started, no plan to start without a separate design pass.

## xolu-client-ext

### T-21. Client.Health() still doesn't apply auth — re-filed, only remaining item from the original xolu request

Theme: xolu-client-ext · Priority: P2 · Status: ☐ · Blocks/after: After: none, standalone

The only item from the original docs/xolu-requests.md not addressed by xolu v0.25.0 (confirmed by direct re-inspection of pkg/client/client.go's Health() -- unchanged, still never sets an Authorization header). xoluman's 'Test connection' and 'Test before saving' (v0.6.6) can therefore still only confirm the server is reachable, never that the configured credential is actually valid -- a connection with a wrong or expired token looks identical to a correctly-configured one. Re-file as its own focused ask to the xolu team rather than letting it stay buried as one item in a since-closed six-item document.

### T-23. xolu client bug: GetEntitySchema/DefineEntitySchema wrongly tenant-prefixed — workaround shipped, request filed

Theme: xolu-client-ext · Priority: P1 · Status: ◐ · Blocks/after: After: none. Waiting on xolu team response to docs/xolu-requests-tenant-schema.md.

Real, severe bug: any xoluman connection with a tenant configured was completely broken for entity browsing — every GetEntitySchema call (Show, EditForm, NewForm, Create, Update, GridView, ImportPreview) failed with XOLU-ST004 'Invalid ID'. Root cause confirmed precisely: Client.buildURL applies the tenant path prefix to every request once a tenant is set, with no per-endpoint awareness; /schema/{entity} is registered on the server only at the global level (confirmed against pkg/server/server.go's own route table), never duplicated under the tenant router, unlike /entities, schema-suggestion, and both promote endpoints which genuinely are. A tenant-scoped client's schema fetch for 'companies' therefore requests /api/v1/tenant/{tenant}/schema/companies -- xolu's router matches this against the entity-by-id pattern instead, landing 'companies' in the numeric {id} slot, failing strconv.Atoi. Reproduced byte-for-byte via direct curl before touching any xoluman code, using xolu's own examples/crm demo (the realistic, tenant-scoped, multi-entity-type dataset that finally surfaced it after several sessions of not being able to reproduce with tenant-less test data). Fixed in xoluman: internal/xoluext.BuildSchemaClient() builds a second, tenant-less client per connection used only for the two schema calls; all 6 real call sites migrated (Show, resolveEntityFields covering NewForm/Create, EditForm, Update, GridView, ImportPreview); 2 call sites (GridView, ImportPreview) had their now-unnecessary regular clientFor call removed entirely rather than left unused. 7 new tests. Verified end-to-end against examples/crm: the exact previously-failing request now returns 200 with correct data; a full sweep across all 6 entity types' list pages, edit pages, and ref links (121 URLs) found zero failures. Request filed to the xolu team (docs/xolu-requests-tenant-schema.md) since the correct long-term fix is in the client library itself; xoluman's workaround is not something to keep maintaining once that lands.

## import-export

### T-22. Backup/export UI feature using xolu v0.25.0's Client.Export

Theme: import-export · Priority: P2 · Status: ☐ · Blocks/after: After: none, xolu v0.25.0 Client.Export available now

The actual xoluman-side feature T-02 existed to unblock -- T-02 itself is closed (client method now exists, delivered by the xolu team, redesigned as async/tenant-scoped/blob-backed rather than the originally-scoped synchronous stream, for a real security reason: the old GET /api/v1/export had zero tenant scoping). Client.Export(ctx, w io.Writer) (*ExportResult, error) hides the async polling entirely -- one call, same experience as the original synchronous design would have had. Design needed before implementation: where does the download trigger live (a button on the connection row, matching Test/Delete's placement, most likely); does xoluman stream the download straight through to the browser as the HTTP response (simplest, no server-side temp file) or write to a temp file first (only needed if some intermediate step, like showing a completion message with file size, is wanted -- probably not needed for v1, stream straight through). Given Export's own polling can take a while on a large tenant, the HTTP handler triggering it needs no client-facing timeout shorter than xolu's own -- check whether context.Background() with no deadline (matching how long-running operations are already handled elsewhere, e.g. import) is right here too, or whether a generous-but-real timeout is worth adding so a truly stuck export doesn't hang the request forever.

