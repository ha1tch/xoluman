# Vendored assets

Everything in this directory is vendored locally so xoluman runs fully
disconnected — no CDN or other live internet dependency at runtime,
since it manages xolu instances that may themselves be on air-gapped or
disconnected local networks. See `docs/KNOWN_ISSUES.md`'s recorded
decision. All frontend dependencies are embedded into the Go binary via
`//go:embed` (`web/embed.go`); no network access is required to serve
the UI after the binary is built.

**Node.js is a build-time dependency only** — used to run the Tailwind
CLI and, occasionally, `esbuild` to bundle a vendored file. It is never
required at runtime.

This document follows the same format Seam AMS uses for its own
`web/static/vendor/VENDOR.md` — reused deliberately rather than
inventing a different one.

---

## Asset 1 — htmx

| Field | Value |
|---|---|
| Vendored file | `htmx@1.9.10.min.js` |
| npm package | `htmx.org` version 1.9.10 |
| License | BSD 2-Clause |
| Size | 47,755 bytes |
| SHA-256 | `b3bdcf5c741897a53648b1207fff0469a0d61901429ba1f6e88f98ebd84e669e` |
| Used in | Every page — `internal/ui/layout.go`'s `Page` shell |
| Replaces CDN | `https://unpkg.com/htmx.org@1.9.10` |

**Upstream canonical URLs:**
```
npm tarball:   https://registry.npmjs.org/htmx.org/-/htmx.org-1.9.10.tgz
npm homepage:  https://www.npmjs.com/package/htmx.org/v/1.9.10
GitHub:        https://github.com/bigskysoftware/htmx/releases/tag/v1.9.10
```

**What it does.** AJAX, CSS transitions, and partial-swap capabilities
via HTML attributes (`hx-get`, `hx-post`, `hx-target`, `hx-swap`).
xoluman uses it for the "Test connection" status swap and the shared
modal's form-loading pattern.

**Air-gap impact if missing.** Every page is non-functional. Not
optional.

**Re-vendoring:**
```bash
curl -sL "https://unpkg.com/htmx.org@1.9.10/dist/htmx.min.js" \
    -o web/static/vendor/htmx@1.9.10.min.js
sha256sum web/static/vendor/htmx@1.9.10.min.js
# Update the SHA-256 and size above.
```

---

## Asset 2 — Tabulator

| Field | Value |
|---|---|
| Vendored files | `tabulator@6.5.2.min.js`, `tabulator@6.5.2.min.css` |
| npm package | `tabulator-tables` version 6.5.2 |
| License | MIT |
| Size | 445,984 bytes (JS), 28,496 bytes (CSS) |
| SHA-256 (JS) | `04802e757fa4189342c666d0f970a01d761c312798f31ffc664c24cbccc7ce3e` |
| SHA-256 (CSS) | `b55e204b2f968cecc4d3663d37858093b31dd22d20f01d76f590726ee18f7e1f` |
| Used in | Not yet wired into any page — vendored ahead of the grid editor's Lit-shell integration (T-11, 2026-08-03) |
| Replaces CDN | `https://unpkg.com/tabulator-tables@6.5.2/dist/...` |

**Upstream canonical URLs:**
```
npm tarball:   https://registry.npmjs.org/tabulator-tables/-/tabulator-tables-6.5.2.tgz
npm homepage:  https://www.npmjs.com/package/tabulator-tables/v/6.5.2
GitHub:        https://github.com/olifolkerd/tabulator/releases/tag/6.5.2
```

**What it does.** A vanilla-JS interactive data grid — sorting,
filtering, resizable columns, inline cell editing. Chosen over Glide
Data Grid specifically because it has no framework dependency (Glide
Data Grid requires React; confirmed, not assumed — see T-11 in
`docs/TRACKING.md`), matching xoluman's no-React rule.

**Air-gap impact if missing.** The grid editor module fails once built.
No other page is affected.

**Re-vendoring:**
```bash
curl -sL "https://unpkg.com/tabulator-tables@6.5.2/dist/js/tabulator.min.js" \
    -o web/static/vendor/tabulator@6.5.2.min.js
curl -sL "https://unpkg.com/tabulator-tables@6.5.2/dist/css/tabulator.min.css" \
    -o web/static/vendor/tabulator@6.5.2.min.css
sha256sum web/static/vendor/tabulator@6.5.2.min.*
# Update the SHA-256s and sizes above.
```

---

## Asset 3 — Lit

| Field | Value |
|---|---|
| Vendored file | `lit@3.js` |
| npm package | `lit` version 3.2.1 |
| Build tool | esbuild 0.21.5 |
| License | BSD 3-Clause |
| Size | 16,052 bytes (minified ESM bundle) |
| SHA-256 | `2363a5c2aea6f202bacd106acea163912eb0e892576742e1156edfd9fc28488b` |
| Used in | Not yet wired into any page — vendored ahead of the grid editor's Lit shell (T-11), and reserved for the FSM/graph editors when they land |
| Replaces CDN | `https://esm.sh/lit@3` |

**Upstream canonical URLs:**
```
npm tarball:   https://registry.npmjs.org/lit/-/lit-3.2.1.tgz
npm homepage:  https://www.npmjs.com/package/lit/v/3.2.1
GitHub:        https://github.com/lit/lit/releases/tag/lit%403.2.1
CDN (esm.sh):  https://esm.sh/lit@3.2.1
```

**What it does.** A lightweight Web Components library. Wraps each of
xoluman's richer embedded widgets (grid editor, and later the graph and
FSM editors) in a consistent shell — toolbar, theming, canvas/component
lifecycle — around whatever framework-agnostic engine each one needs
(Tabulator for the grid; TBD for graph/FSM). Same architecture Seam AMS
uses for its own FSM editor.

**Why the npm package can't be copied directly.** The `lit` package
uses sub-path exports and ships as multiple files — it cannot be served
as a single import without bundling. Version and build tool are pinned
to exactly match Seam AMS's own already-validated recipe for this asset
(same npm and esbuild versions) rather than picking the latest
independently — there's no reason to introduce a second, unvalidated
combination when a working one already exists.

**Air-gap impact if missing.** Any page using a Lit-shelled widget
fails. Other pages are unaffected.

**Re-vendoring:**
```bash
npm install lit@3.2.1 esbuild@0.21.5
npx esbuild --bundle --format=esm --minify \
    --outfile=web/static/vendor/lit@3.js \
    node_modules/lit/index.js
sha256sum web/static/vendor/lit@3.js
# Update the SHA-256 above. lit and esbuild are build-time-only —
# remove them from node_modules afterward; neither belongs in
# package.json as a tracked devDependency (only tailwindcss is).
```

---

## Asset 4 — CodeMirror 6 (query editor: OQL, Sulpher, REST)

| Field | Value |
|---|---|
| Vendored file | `codemirror-bundle@1.js` |
| npm packages | `codemirror` 6.0.2, `@codemirror/lang-sql` 6.10.0, `@codemirror/lang-json` 6.0.2, `@neo4j-cypher/codemirror` 1.0.3 (plus their own transitive `@codemirror/*`/`@lezer/*` dependencies, all MIT) |
| Build tool | esbuild 0.21.5 |
| License | MIT (CodeMirror core and official language packages), Apache-2.0 (`@neo4j-cypher/codemirror` and its `@neo4j-cypher/editor-support` dependency) — confirmed per-package, not assumed from the design note that first proposed this |
| Size | 1,204,962 bytes (minified ESM bundle) |
| SHA-256 | `2d5301fea8f3c7479f02b4d3596462dfa2c9d40d736bb9dc2154946bb17d75bd` |
| Used in | T-12's query editor (`internal/ui/query.go`, `web/static/js/query-editor.js`) |
| Replaces CDN | N/A — CodeMirror has no CDN-equivalent single-file distribution; see below |

**Upstream canonical URLs:**
```
npm (codemirror):              https://www.npmjs.com/package/codemirror/v/6.0.2
npm (@codemirror/lang-sql):    https://www.npmjs.com/package/@codemirror/lang-sql/v/6.10.0
npm (@codemirror/lang-json):   https://www.npmjs.com/package/@codemirror/lang-json/v/6.0.2
npm (@neo4j-cypher/codemirror): https://www.npmjs.com/package/@neo4j-cypher/codemirror/v/1.0.3
GitHub (CodeMirror):           https://github.com/codemirror/dev
GitHub (Neo4j Cypher editor):  https://github.com/neo4j/cypher-editor-support
```

**What it does.** A real code editor (not just a highlighter — cursor,
typing, selection, autocomplete) for xoluman's three query modes: OQL
(a T-SQL subset — `lang-sql`'s MSSQL dialect is safe to use directly,
since anything valid OQL is valid T-SQL by construction, zero custom
grammar needed), Sulpher (near-exactly openCypher9 — Neo4j's own real
ANTLR4 Cypher grammar plus semantic analysis, not a regex
approximation), and REST request bodies (`lang-json`).

**Why the npm packages can't be copied directly.** CodeMirror 6's
architecture is deliberately modular — a dozen-plus small packages
(`@codemirror/state`, `@codemirror/view`, `@codemirror/language`,
`@codemirror/commands`, `@codemirror/autocomplete`, `@codemirror/lint`,
`@codemirror/search`, the `@lezer/*` parser runtime, and so on) that
only resolve against each other through real module imports — there is
no single-file distribution to fetch, same underlying reason as Lit
(Asset 3). Bundled with esbuild, same recipe shape.

**Air-gap impact if missing.** The query editor page fails to load its
editor entirely (plain `<textarea>` fallback not currently
implemented). No other page is affected.

**Re-vendoring:**
```bash
npm install codemirror@6.0.2 @codemirror/lang-sql@6.10.0 \
    @codemirror/lang-json@6.0.2 @neo4j-cypher/codemirror@1.0.3 esbuild@0.21.5
mkdir -p .buildtmp
cat > .buildtmp/codemirror-entry.js << 'EOF'
export { EditorView, basicSetup } from 'codemirror';
export { EditorState, Compartment } from '@codemirror/state';
export { sql, MSSQL } from '@codemirror/lang-sql';
export { json } from '@codemirror/lang-json';
export { getExtensions as cypherExtensions } from '@neo4j-cypher/codemirror';
EOF
npx esbuild .buildtmp/codemirror-entry.js --bundle --format=esm --minify \
    --outfile=web/static/vendor/codemirror-bundle@1.js
rm -rf .buildtmp
sha256sum web/static/vendor/codemirror-bundle@1.js
# Update the SHA-256 and size above. All packages here are build-time-only
# — remove them from node_modules afterward, same as Lit's own note.
```

---

## License summary

| Asset | License | Attribution required | Notes |
|---|---|---|---|
| htmx | BSD 2-Clause | Yes | Retain copyright notice |
| Tabulator | MIT | Yes | Retain copyright notice |
| Lit | BSD 3-Clause | Yes | Retain copyright, no endorsement clause |
| CodeMirror + language packages | MIT | Yes | Retain copyright notice |
| `@neo4j-cypher/codemirror` | Apache-2.0 | Yes | Retain copyright, state changes if modified (not modified here) |

All licenses are permissive. No copyleft obligations. Attribution is
required in source-code distribution but not in compiled binaries —
these files ship as embedded binary data inside a compiled Go binary
via `go:embed`, which falls outside source-distribution requirements.
This file itself is the attribution record.

---

## Verifying vendored files

```bash
cd web/static/vendor
sha256sum -c <<'HASHEOF'
b3bdcf5c741897a53648b1207fff0469a0d61901429ba1f6e88f98ebd84e669e  htmx@1.9.10.min.js
04802e757fa4189342c666d0f970a01d761c312798f31ffc664c24cbccc7ce3e  tabulator@6.5.2.min.js
b55e204b2f968cecc4d3663d37858093b31dd22d20f01d76f590726ee18f7e1f  tabulator@6.5.2.min.css
2363a5c2aea6f202bacd106acea163912eb0e892576742e1156edfd9fc28488b  lit@3.js
2d5301fea8f3c7479f02b4d3596462dfa2c9d40d736bb9dc2154946bb17d75bd  codemirror-bundle@1.js
HASHEOF
```
