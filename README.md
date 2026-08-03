# xoluman

**A web-based operator UI for xolu: manage instances, browse and edit
data, run queries, and move data in and out.**

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8.svg)](https://golang.org/)

> **v0.0.1** — scaffolding stage, no functionality shipped yet.

---

## What xoluman is

xoluman is a single Go binary, server-rendered with
[minty](https://github.com/ha1tch/minty) and htmx — no hand-written HTML,
no React. It talks to one or more [xolu](https://github.com/ha1tch/xolu)
instances (local or remote) through xolu's own Go client library
(`github.com/ha1tch/xolu/pkg/client`), and adds what a single xolu
instance doesn't provide for itself:

- **Connections** — a registry of named xolu instances (local and
  remote), each with its own base URL, auth mode, token, and default
  tenant. Switch between them without restarting.
- **Data browser/editor** — enumerate entity types, view and edit
  entities through a schema-driven form built from xolu's own JSON
  Schema, no per-entity-type code required.
- **Query runner** — OQL, Sulpher, and ad hoc REST requests against the
  active connection, with a results view.
- **/blob access** — put, get, list, and inspect usage for the blob
  primitive.
- **Import/backup** — trigger a whole-database backup snapshot
  (`GET /api/v1/export`); import entities from CSV/JSON where xolu has
  no native import endpoint of its own.
- **Export** — render the current query or listing results to CSV, XLSX,
  or ODS.

xoluman does not duplicate xolu's data model or query engines — it is a
thin, generic operator surface over whatever xolu instance is active.

## Status

This repository is at the scaffolding stage: tracking documents,
tooling, and the connection-store design are in place; the UI and most
features are not yet built. See [docs/TRACKING.md](docs/TRACKING.md)
for the live register of open work.

## Relationship to xolu and Seam AMS

xoluman depends on `github.com/ha1tch/xolu/pkg/client` for all
communication with xolu instances — it adds no parallel HTTP layer of
its own beyond what that client doesn't yet cover (blob, export, raw
REST — tracked as T-01/T-02/T-03).

xoluman's UI conventions (minty, htmx, server-rendered layout and
components) follow the same practice established in Seam AMS, trimmed
to what a generic operator tool needs: no tenant-specific business
modules, no asset domain model, no tabs/visibility-rule form engine —
just flat, schema-driven forms over arbitrary entity types.

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

## License

Copyright (c) 2026 haitch

Apache 2.0 — see [LICENSE](LICENSE).
