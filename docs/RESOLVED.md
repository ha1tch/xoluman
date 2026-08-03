# xoluman — Resolution Record

Append-only, newest first. Closed items are moved here verbatim from
`TRACKING.md`, stamped with closing version and date, per the closure
procedure.

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

