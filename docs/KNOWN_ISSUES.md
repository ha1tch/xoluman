Version: 0.6.4
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

## Dormant guards

| Guard | Gating condition | Hardware/environment requirement | Canonical invocation | Last exercised |
|---|---|---|---|---|
| Keyring backend round-trip | Requires an OS keyring service | Not available in the sandbox container; needs Horacio's local machine | `go test ./internal/connstore/... -tags keyring_live -count=1` | Written and confirmed failing informatively (no keyring service) in the sandbox, 2026-08-03 — `exec: "dbus-launch": executable file not found in $PATH`. Not yet run against a real keyring service. |
