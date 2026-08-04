Version: 0.6.17
Last reviewed: 2026-08-03

# xoluman — Known Issues and Recorded Decisions

Intentional limits and invariant boundaries. Open actionable work lives
in `TRACKING.md`, not here.

## Recorded decisions

- **xoluman does not modify xolu directly (2026-08-03).** Changes to
  `xolu/pkg/client` — or anything else in xolu — are requests to the
  xolu team, written up as plain-language documents (`docs/xolu-requests.md`),
  not code this project writes into Horacio's local xolu checkout. The
  `go.mod` `replace github.com/ha1tch/xolu => ...` directive exists so
  xoluman can build against a local copy; it is not licence to edit that
  copy. Established after T-01 was incorrectly implemented and closed
  directly against the local checkout, caught by Horacio, and discarded
  — see T-01's correction note in `docs/RESOLVED.md` and T-13 in
  `TRACKING.md` for the actual request this produced.
- **Disconnected operation is a hard requirement (2026-08-03).**
  xoluman must run fully disconnected — no CDN or other live internet
  dependency at runtime — because it manages xolu instances that may
  themselves be on air-gapped or otherwise offline local networks. This
  is why Seam vendors Tailwind/Material Icons/htmx/Lit rather than
  pulling them from a CDN; it was previously mischaracterized in this
  project's own notes as being about avoiding "bloat," which is wrong —
  it's about working without internet access at all. Concretely: every
  JS/CSS dependency xoluman ever adds is vendored under
  `web/static/vendor/` and embedded into the binary via `web/embed.go`
  (`go:embed`), never referenced by CDN URL. `TestPage_NoExternalCDNReferences`
  (`internal/ui/listing_test.go`) is the regression guard.
- **Styling: adopted Seam's actual Tailwind system, not a substitute
  (2026-08-03).** First pass at the UI shell built a small custom
  inline-CSS stylesheet instead of Tailwind, reasoning it was "avoiding
  bloat" Seam's design system didn't need — wrong on two counts: that
  wasn't what vendoring was for (see above), and inventing a new,
  unrefined styling approach instead of reusing Seam's proven one was
  strictly more work, not less. Corrected: `package.json` +
  `tailwind.config.js` + `scripts/tailwind.input.css` mirror Seam's own
  setup exactly (Tailwind 3.4.19, JIT content-scanning over
  `internal/**/*.go`, `darkMode: 'class'`). `make css` / `npm run css`
  compiles `web/static/css/tailwind.css`, which is committed (not
  gitignored — `go:embed` needs it present at build time) and embedded
  into the binary, never fetched at runtime. Button/table/form classes
  in `internal/ui/listing.go` and `connections.go` match Seam's own
  conventions (`bg-indigo-600 ... hover:bg-indigo-700` for primary,
  etc.) rather than inventing new ones.
- **Self-check note:** htmx itself shipped on a CDN URL in T-10's first
  pass despite this being the explicit stated goal — caught when asked
  directly, not by self-review. Fixed same session (`htmx@1.9.10.min.js`
  now vendored, see `web/static/vendor/VENDOR.md`). Worth remembering
  that a stated principle needs to be checked against what was actually
  built, not assumed to have been followed. The Tailwind reversal above
  is the same pattern again: reasoning about a decision in the abstract
  ("this counts as bloat") instead of checking what the decision was
  actually for.
- **Connection secret storage (2026-08-03).** `ConnectionStore` supports
  two backends — plaintext JSON (git-ignored, `0600`) and OS keyring.
  The backend is a single xoluman-level setting chosen once at first
  run, not a per-connection choice. See T-06/T-04 in `TRACKING.md`.
- **No native xolu import endpoint.** xolu's `/api/v1/export` is a
  whole-database backup snapshot with no corresponding import endpoint.
  xoluman's import feature is therefore built on entity-by-entity
  `Create`/`Commit` calls, not a server-side restore. See T-05.
- **Internal documentation language.** All new internal documentation
  (specs, plans, tracking docs, architecture notes) is written in
  English, no exception — same invariant as Seam AMS, adopted from the
  start here rather than retrofitted.
- **UI shell: module registry, no RBAC (2026-08-03).** xoluman adopts
  Seam's module-registry pattern (self-registering modules, nav built
  from the registry) now that graph editor and FSM editor modules are
  confirmed as planned, alongside the data editor. It does not adopt
  Seam's role-based `VisibleTo(role)` gating — xoluman has no
  multi-user/role model. See T-08.
- **Blob browser: virtual hierarchy, entity-backed (2026-08-03).**
  xolu blob keys cannot contain `/`, enforced even on the S3-compatible
  surface — no hierarchy exists in xolu itself. xoluman presents one
  using `:` as an in-key delimiter, translated only at the UI boundary,
  with empty folders and drift-healing backed by a `xoluman_blob_folder`
  entity (REF-linked parent chain, no stored path) inside the target
  xolu instance itself — not a xoluman-local index. See T-09.
- **Internal vs external identifier language.** Internal = identifiers,
  invariants, constants, states, not visible to an end user — always
  English, no exception. This covers enum *values* used as validation
  constants or stored state, not just field/identifier names. External
  = anything user-visible — follows locale.
- **xolu's `/api/v2` surface is disabled by default.** Discovered
  during T-14's FSM def persistence e2e verification: `/api/v2/...`
  requests 404 with plain "page not found" (not an XOLU-coded error)
  unless the target xolu instance was started with
  `XOLU_API_V2_ENABLED=true` — confirmed in xolu's own
  `pkg/server/v2_handlers.go`, a deliberate choice on xolu's side (404
  over 501, so "no v2 here" reads unambiguously). Affects FSM
  definitions and anything else xoluman builds against `/api/v2` —
  worth remembering when a v2-backed feature "can't find" something
  that plainly exists; check this before anything else. A plain
  `http.NotFound`-shaped 404 (not an `xclient.Error`/XOLU-coded one) on
  a v2 endpoint is close to a diagnostic signature for this specific
  cause.
- **xolu FSM guard expressions are T-SQL syntax, not C-style.**
  Confirmed the same session: `key_valid == true` fails to parse
  (`XOLU-FSM011`); `key_valid = true` is correct — single `=` for
  equality, matching the T-SQL-via-`tsqlparser` expression language
  fsm-toolkit's own guard-support feature request settled on. Also
  confirmed: `Determinism` only accepts `"strict"`, `"loose"`, or
  `"firstmatch"` (required, no default); a transition's `Output` must
  already appear in `MachineSpec.OutputAlphabet` or validation rejects
  it with `XOLU-FSM006`.
- **xolu REF field values are not what a first read of the field's
  name suggests, and this was wrong for the whole life of the ref-
  navigation feature (T-15) until a real end-to-end write-then-read
  test caught it.** Three genuinely different shapes are involved, not
  one:
  - **Write**: xolu requires the structured shape
    `{"type":"REF","entity":"<target>","id":<int>}` — a bare integer is
    rejected outright with `XOLU-VL001` ("expected {type,entity,id},
    got float64"), confirmed by trying it directly against a real
    server.
  - **Read via `GET /{entity}/{id}` (single entity)**: the *entire
    resolved target document* is embedded in place of the reference —
    `{"id":N,"name":"...",...every field the target document has}` —
    not a stub with just an ID, and not the write shape either.
  - **Read via `GET /{entity}` (list)**: returns the raw stored write
    shape (`{"type":"REF","entity":"...","id":N}`), *not* resolved —
    confirmed by comparing single-entity GET and LIST responses for
    the same row side by side. This asymmetry appears intentional
    (resolving every row's refs in a paginated list would be a real
    per-row cost xolu evidently avoids by design), and it means a ref
    field's display label is only available for free where GET already
    did the resolving (a single entity's edit form) — the list preview
    correctly has no label to show without a separate fetch per row,
    which was already the deliberate, documented reasoning for keeping
    the list preview to raw IDs, now doubly confirmed as necessary
    rather than just a reasonable simplification.
  Every part of xoluman that touches a ref field's value —
  `formengine`'s edit-form rendering and parsing, `internal/ui/refs.go`'s
  link and label resolution, the entity list preview, and CSV/JSON
  import — was corrected for all three shapes and re-verified against
  a real server end to end, not just re-mocked.
- **Schemas are not obligatory in xolu — entity types can exist purely
  as data, with no registered schema at all.** This was missed for the
  whole life of the entity browser (T-07/T-10) until a real user report
  ("no entities appear") traced back to it. Confirmed directly:
  `POST /api/v1/{entity}` succeeds and the data is fully readable via
  `GET /api/v1/{entity}` with zero schema ever registered — but
  `GET /api/v1/schemas` (`handleListSchemas` → `validator.LoadedEntities()`)
  reflects *registered schemas only*, nothing to do with what data
  actually exists, and `GET /api/v1/schema/{entity}` 404s
  (`XOLU-ST008`) for an unregistered type. Every xoluman entity page
  called `GetEntitySchema` first and treated any failure as fatal,
  making schema-less entity types completely invisible and
  unbrowsable, even though the data was real and reachable the whole
  time. Fixed in two passes: T-17 (xolu v0.25.0's `ListEntities`
  replaced the discovery gap properly) and T-16 (`NewForm`/`Create`/
  `EditForm`/`Update` now infer fields from real data when there's no
  schema, rather than failing outright).
- **Query editor mode switching (`web/static/js/query-editor.js`) shared
  one CodeMirror document across all three modes — a real bug, found
  from a direct user report.** `_setMode` only reconfigured the
  language extension (`this._languageConf.reconfigure(...)`), never
  the document itself, so text typed in one mode was still there,
  under a different language's highlighting, after switching to
  another mode — reported precisely as "the OQL tab contains the
  Sulpher query I was editing previously." Fixed by keeping each
  mode's document separately and doing a full `EditorView.setState()`
  on switch (fresh `EditorState.create()` with the incoming mode's own
  saved text) rather than an incremental reconfigure.
- **Sulpher (Cypher) syntax highlighting reported not appearing at
  all, while OQL and REST/JSON highlight correctly.** Traced the full
  extension chain from `@neo4j-cypher/codemirror`'s `getExtensions()`
  down to `getCypherLanguageExtensions` and its bundled `syntaxCSS`
  (`[syntaxHighlighting(syntaxStyle)]`) — structurally correct on
  paper, `cypherLanguage: true` by default, nothing obviously
  misconfigured. Could not conclusively diagnose further by reading
  code alone; this sandbox has no real browser to visually confirm
  against. The mode-switching fix above (full `setState()` instead of
  `reconfigure()` on every mode change) may resolve this as a side
  effect, since it eliminates any stale Compartment state from
  incremental reconfiguration — but that's inference, not a confirmed
  fix. Flagged honestly rather than claimed fixed; needs a real-browser
  check.
- **xolu's blob storage is disabled by default too — same category of
  gotcha as `XOLU_API_V2_ENABLED`.** Found while building and verifying
  T-09's blob browser: every `/api/v1/blob` call fails with a clear
  `501` (`"Blob storage is not enabled on this server
  (XOLU_BLOB_ENABLED=true required)"`) unless the server was started
  with `XOLU_BLOB_ENABLED=true`. Unlike the v2-API gate, at least this
  one fails with an unambiguous, correctly-coded error rather than a
  plain 404 that could be mistaken for something else — but it's easy
  to lose ten minutes wondering why blob calls fail on a "default
  settings" instance if this isn't already known going in.

## Dormant guards

| Guard | Gating condition | Hardware/environment requirement | Canonical invocation | Last exercised |
|---|---|---|---|---|
| Keyring backend round-trip | Requires an OS keyring service | Not available in the sandbox container; needs Horacio's local machine | `go test ./internal/connstore/... -tags keyring_live -count=1` | Written and confirmed failing informatively (no keyring service) in the sandbox, 2026-08-03 — `exec: "dbus-launch": executable file not found in $PATH`. Not yet run against a real keyring service. |
