# Requests from xoluman to the xolu team

Author: Horacio, via Claude (xoluman development session)
Date: 2026-08-03
Status: draft — for the xolu team's review, not a commitment to anything

---

## What this document is

xoluman is a web-based operator UI for xolu — connection management,
data browsing/editing, query running, and blob access, against one or
more xolu instances. It depends entirely on `xolu/pkg/client`.

This document lists everything xoluman currently needs from xolu that
xolu doesn't yet provide, or provides with a gap. It's a request, not a
patch — none of this has been implemented against your working copy.
Each item says what's needed, which xoluman feature it blocks, and why
it can't reasonably be built xoluman-side instead.

This was written after a deliberate review of the whole project's
requirements, not just whatever came up first — the last section lists
things that were considered and are *not* being requested, so the scope
here is what's actually load-bearing, not a wishlist.

---

## 1. Blob primitive client methods

**Blocks:** xoluman's blob browser (a directly-requested feature from
the original spec: "provide easy access to the `/blob` primitive"), and
its virtual-hierarchy design on top of it.

`docs/BLOB_API.md` documents a full REST surface —
`POST/GET/HEAD/DELETE /api/v1/blob{,/key}`, `GET /api/v1/blob` (list),
`GET /api/v1/blob/usage` — none of which `pkg/client` currently wraps.
Everything else in xoluman builds on typed client methods
(`Create`/`Get`/`Update`/`List`/…); the blob primitive is the one gap.

Requesting: `BlobPut`, `BlobGet`, `BlobHead`, `BlobDelete`, `BlobList`,
`BlobUsage`, tenant-scoped like the rest of the client, mirroring the
existing method style.

One implementation note worth passing along: blob transfer doesn't fit
the existing `do`/`doURL`/`doOnce` helpers — they hardcode
`Content-Type: application/json`, which is wrong for arbitrary blob
content, and have no header-injection point for `X-Blob-Key`. Whoever
picks this up will likely want a separate low-level path for these
methods rather than forcing them through the JSON-CRUD helpers.

---

## 2. A streaming Export method

**Blocks:** xoluman's backup feature (also directly requested in the
original spec).

`GET /api/v1/export` streams a zip (manifest + database file + optional
`graph.json`), and `EXPORT_API.md` is explicit that it streams without
writing a server-side temp file — meaning nothing bounds the response
size in advance. A client method needs to write to an `io.Writer`, not
buffer into `[]byte`, or a large database turns into a large in-memory
allocation on every caller.

**A documentation bug found in the course of reviewing this:**
`EXPORT_API.md` says the database file inside the zip is named
`entities.db`, with manifest key `entities_file`, plus a `graph_files`
array key. The actual handler
(`pkg/server/handlers.go:handleExport`) writes the file as `xolu.db`
and sets the manifest key `database_file` — confirmed by reading the
handler directly, not just the doc. There's no `graph_files` key at
all; only `graph_json` when the graph subsystem is enabled. Worth
fixing the doc regardless of when/whether the client method lands,
since anyone hand-parsing the manifest against the current doc will
get it wrong.

Also worth knowing for whoever implements the client side: `/export`
is not tenant-scoped — confirmed via `server.go`'s route registration,
it's registered only in the non-tenant-routes block and is disabled
entirely under `XOLU_TENANT_MODE=strict`. The client's usual
`buildURL` tenant-prefixing must not apply here, or a client configured
with a tenant would construct a URL that doesn't exist server-side.

---

## 3. A minimal raw request method

**Blocks:** xoluman's ad hoc REST query console (one of three query
modes alongside OQL and Sulpher, both already fully covered by
existing `Client.OQL`/`Client.Sulpher`).

`Client.do`/`doURL`/`doOnce` are all unexported. An operator tool that
wants to let someone issue an arbitrary method+path+body request
against a connected instance — using whatever auth is already
configured, without re-implementing auth/retry logic client-side — has
no way to do that today. Requesting something like:

```go
Raw(ctx context.Context, method, path string, body io.Reader) (status int, respBody []byte, err error)
```

or whatever shape fits best. The point isn't the exact signature, just
that the existing auth/retry logic lives in one place rather than being
reimplemented by every consumer that wants raw access.

---

## 4. A client wrapper for schema registration

**Blocks:** part of xoluman's blob-hierarchy design (T-09) — not
blob access itself (covered by #1), but the "virtual folders" layer on
top of it.

`docs/JSON_SCHEMA.md` documents `POST /api/v1/schema/{entity}` as a
real, working endpoint — confirmed by reading the doc directly, and
confirmed that `pkg/client` has no method wrapping it (`GetEntitySchema`
exists for reading; nothing exists for writing). xoluman's blob-folder
design needs to register a schema for a small bookkeeping entity type
(`xoluman_blob_folder`) it creates inside whichever xolu instance is
being browsed, so that entity type gets proper validation and shows up
correctly through xoluman's own generic schema-driven editor, the same
as any other entity type.

Worth being upfront: schema registration has real side effects per your
own docs — it creates an adapted table and starts enforcing validation
immediately. Whatever shape this method takes is the xolu team's call,
not something to design from outside. Flagging the need, not proposing
an API.

---

## 5. `Client.Health()` doesn't apply the configured auth header

**This isn't a missing method — it's a behavioural gap in something
that already exists**, and it affects xoluman functionality that has
already shipped.

Confirmed by reading `Health()` directly: it builds its own request and
never calls `authHeader()`. xoluman's connection-management page has a
"Test connection" button built on `Health()` — right now, that button
can only tell someone whether the *server* is reachable, not whether
the *token* they configured is actually valid. A connection with a
wrong or expired API key currently looks identical, from `Health()`'s
perspective, to one that's perfectly configured.

This is a real problem for a tool whose whole purpose is managing
connections and their credentials — the one thing "test this
connection" most needs to check is exactly the thing it currently
can't. Two ways this could go, and no preference expressed here since
it's genuinely the xolu team's design call:

- Make `Health()` (or a variant) apply the configured auth header, so
  an invalid credential surfaces as a distinct failure from server
  unreachability, or
- Point us at whichever existing authenticated, cheap endpoint is the
  right thing for a client to hit purely to validate "is this server
  reachable *and* is this credential accepted" — `V2Availability` was
  considered as a candidate but its auth behaviour and cost weren't
  verified, so it's mentioned as a question, not a recommendation.

---

## 6. A smaller inconsistency, worth a quick look

`client.FieldDef.Type`'s doc comment lists `"ref"` as a possible `Type`
value, but the actual schema-extraction code
(`extractFieldsFromSchema`) only ever sets it via `Format == "ref"`,
never via `Type`. xoluman's form renderer defensively handles both
cases, so this isn't blocking anything — just flagging that either the
comment is stale, or there's a code path setting `Type == "ref"` that
wasn't found during review. Worth a five-minute confirm either way.

---

## Reviewed and *not* requesting

Listed so the scope above reads as reviewed, not assembled by grepping
for the first gap found in each area:

- **Bulk/multi-entity import beyond `Commit`.** `POST /api/v1/commit`
  already covers atomic multi-entity writes, which is enough for
  reasonably-sized import batches. Not asking for a dedicated bulk
  import endpoint at this time — xoluman's import feature (T-05) is
  designed around `Create`/`Commit` as they exist. Might revisit if
  that design work surfaces a real limit, but nothing concrete yet.
- **OQL/Sulpher query execution.** Already fully covered by
  `Client.OQL` and `Client.Sulpher`. Nothing needed.
- **Entity CRUD, listing, pagination, search.** Already fully covered
  by `Create`/`Get`/`Update`/`Patch`/`Delete`/`List`/`Search`. Nothing
  needed.
- **Echoing the document back after a write.** `Create`/`Update`/`Patch`
  return `Data: nil` by design, requiring a follow-up `Get` to redisplay
  a saved entity. A minor extra round-trip, not a blocker — not asking
  for a change, just noting it was considered.
- **Bulk grid-editing support** (hundreds of rows/cells at once,
  xoluman's planned T-11). Not concretely designed yet, so nothing
  concrete to ask for. May come back once that design exists, if it
  turns out `Commit` isn't enough for that shape of write.

---

## Priority, from xoluman's side

If it's useful to know which of these matter most for what's currently
blocked:

1. **#1 (blob methods)** and **#5 (Health() auth)** — the first blocks
   a directly-requested feature; the second is a correctness gap in
   something already shipped.
2. **#2 (export)** and **#3 (raw request)** — each blocks one specific,
   already-designed xoluman feature (backup; the REST query console).
3. **#4 (schema registration)** — blocks part of a feature (T-09) that
   isn't the current focus.
4. **#6 (doc/comment inconsistencies)** — no urgency, just worth
   fixing when convenient.

None of this is being implemented xoluman-side against your working
copy. Whatever you decide — including "no" on any of it — xoluman's own
tracking (`docs/TRACKING.md`, item T-13) picks back up from your
response.
