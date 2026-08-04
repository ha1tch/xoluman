# xoluman

**A web-based operator UI for xolu: manage instances, browse and edit
data, run queries, and move data in and out.**

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8.svg)](https://golang.org/)

---

## What xoluman is

xoluman is a single Go binary, server-rendered with
[minty](https://github.com/ha1tch/minty) and htmx — no hand-written HTML,
no React. It talks to one or more [xolu](https://github.com/ha1tch/xolu)
instances (local or remote) through xolu's own Go client library
(`github.com/ha1tch/xolu/pkg/client`), and adds what a single xolu
instance doesn't provide for itself.

xoluman does not duplicate xolu's data model or query engines — it is a
thin, generic operator surface over whatever xolu instance is active.

## What's built

- **Connections** — a registry of named xolu instances, each with its
  own base URL, auth mode, token, and default tenant. Test a
  connection's reachability before saving it, not just after. Switch
  between instances without restarting.
- **Entity browser and editor** — every entity type with actual data
  shows up, schema-registered or not (xolu doesn't require a schema;
  discovery and editing both work either way, inferring fields from
  real data when there's no schema to read them from). REF fields
  render as clickable, resolved-label links, not raw IDs. Bulk import
  from CSV or JSON. A Tabulator-based grid view for editing many rows
  at once.
- **Schema promotion** — preview a heuristic-inferred schema for a
  schemaless entity type, then promote it: strict (validates every
  existing row first, atomic, rejection is a normal outcome with the
  exact failures shown) or flex (fast, with the trade-off — existing
  rows not migrated — stated plainly, not buried).
- **Blob browser** — xolu's blob store presented as a navigable folder
  hierarchy (a xoluman-only convention over xolu's flat key space,
  since xolu keys can't contain `/`). Upload, download, delete, and
  create real empty folders that persist even with nothing in them.
- **Query editor** — OQL, Sulpher, and raw REST requests against the
  active connection, each with real CodeMirror 6 syntax highlighting,
  not a plain textarea.
- **Dark/light theme**, persisted, following system preference by
  default.

## Status

Actively developed. See [docs/TRACKING.md](docs/TRACKING.md) for the
live register of open work, [docs/KNOWN_ISSUES.md](docs/KNOWN_ISSUES.md)
for intentional limits and real gotchas found along the way (several in
xolu itself, not just xoluman), and [CHANGELOG.md](CHANGELOG.md) for
what's actually shipped, release by release.

Not yet built: a backup/export UI (the xolu client method exists;
the xoluman-side feature doesn't yet), and an FSM definition editor
(being worked on separately).

## Relationship to xolu and Seam AMS

xoluman depends on `github.com/ha1tch/xolu/pkg/client` for all
communication with xolu instances — it adds no parallel HTTP layer of
its own. Several client-library gaps found while building xoluman
(blob methods, an async tenant-scoped export, a raw request escape
hatch for the query editor's REST mode, schema registration, and two
bonus items — `ListEntities` and schema promotion) were filed as
requests and delivered upstream in xolu itself, not built as
workarounds inside xoluman.

xoluman's UI conventions (minty, htmx, server-rendered layout and
components) follow the same practice established in Seam AMS, trimmed
to what a generic operator tool needs: no tenant-specific business
modules, no asset domain model, no tabs/visibility-rule form engine —
just flat, schema-driven forms over arbitrary entity types, with a
graceful fallback when there's no schema at all.

## Development workflow

Tooling mirrors xolu's: `repoman` (`repoman/`) for editing, the work
register, and dormant-guard tracking; a `Makefile` for build/test;
tracking documents under `docs/` following the same taxonomy
(`CHANGELOG.md`, `docs/TRACKING.md`, `docs/RESOLVED.md`,
`docs/KNOWN_ISSUES.md`).

```bash
make build      # → ./xoluman
make test       # go test ./...
make test-race  # go test ./... -race
make coverage   # coverage profile + summary
make css        # regenerate web/static/css/tailwind.css after adding
                 # or removing Tailwind classes in Go source (npm install
                 # once first; requires Node.js — build-time only, never
                 # at runtime, see docs/KNOWN_ISSUES.md)
```

All third-party frontend assets (htmx, Tabulator, Lit, CodeMirror) are
vendored under `web/static/vendor/` and embedded via `go:embed` — the
built binary needs no network access to serve its own UI. See
[web/static/vendor/VENDOR.md](web/static/vendor/VENDOR.md) for exact
versions, licenses, and re-vendoring instructions.

## License

Copyright (c) 2026 haitch

Apache 2.0 — see [LICENSE](LICENSE).
