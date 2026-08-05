# Requests from xoluman to the xolu team

Author: Horacio, via Claude (xoluman development session)
First filed: 2026-08-03
Last updated: 2026-08-04
Status: **living document** — this is the one place that tracks every
open and closed request, kept current as things ship or new gaps turn
up, rather than a point-in-time draft. Individual detailed documents
(`docs/xolu-requests-fsm-def.md`, `docs/xolu-requests-tenant-schema.md`,
`docs/xolu-requests-undeclared-ref-update.md`) still hold full
reproduction steps for the items that need them — this letter is the
index and current status, updated to fold all of them in.

Delivered items are struck through with the version that shipped them.
Still-open items are marked **🔴 ACTIVE**. Nothing here has been
implemented against your working copy — xoluman's own tracking
(`docs/TRACKING.md`) is where the corresponding item lives on our side.

---

## What this document is

xoluman is a web-based operator UI for xolu — connection management,
data browsing/editing, query running, and blob access, against one or
more xolu instances. It depends entirely on `xolu/pkg/client`.

This document lists everything xoluman has needed from xolu that xolu
didn't yet provide, or provided with a gap — plus, as of this update,
two real bugs found while actually using the client against realistic
data (`examples/crm`), not just gaps in coverage.

---

## ~~1. Blob primitive client methods~~ — ✅ delivered in v0.25.0

`BlobPut`/`BlobGet`/`BlobHead`/`BlobDelete`/`BlobList`/`BlobUsage`,
confirmed directly against the real v0.25.0 source before being
trusted. Client-side key validation (rejecting `/` and `\`, plus
reserved `.`/`..` and a leading `.`) came as a bonus beyond what was
asked. Powers xoluman's blob browser (T-09), shipped.

## ~~2. A streaming Export method~~ — ✅ delivered in v0.25.0, redesigned

Not the synchronous stream originally asked for — redesigned to async,
tenant-scoped, and blob-backed, for a real reason stated plainly at the
time: the old `GET /api/v1/export` had no tenant scoping at all, a
much bigger blast radius than anything else in the API. `Client.Export`
hides the polling; the caller-facing experience matches what was
originally asked for. The `EXPORT_API.md` doc bug found in the original
ask (`entities.db`/`entities_file` vs. the real `xolu.db`/
`database_file`) — checked against the current source while updating
this letter, not left as a stale assumption: it's fixed, the doc now
correctly says `xolu.db`/`database_file` throughout.

## ~~3. A minimal raw request method~~ — ✅ delivered in v0.25.0

`Client.Raw` — no tenant-prefixing (caller controls the exact path),
no structured-error decoding (caller inspects `StatusCode` directly),
matching what was asked for closely. Powers xoluman's query editor's
REST mode (T-12), shipped and verified end-to-end, including a real
400 from an invalid path coming back as data rather than an error —
exactly the point of an escape hatch.

## ~~4. A client wrapper for schema registration~~ — ✅ delivered in v0.25.0

`Client.DefineEntitySchema`. Not yet consumed by xoluman directly (the
blob-folder design ended up not needing it — `xoluman_blob_folder` is
created schema-less, matching the same pattern as `xoluman_field_meta`)
but available.

**Bonus, not requested:** `ListEntities` and the whole schema-promotion
surface (`GetSchemaSuggestion`/`PromoteFlex`/`PromoteStrict`) — both
shipped in v0.25.0 and both now load-bearing in xoluman (T-17's real
fix for schema-less entity discovery, T-20's promotion UI).

## ~~5. `Client.Health()` doesn't apply the configured auth header~~ — ✅ delivered in v0.27.0, new method

Not a case of "just add an auth header" — `/health` is deliberately
exempt from auth server-side (same convention as `/ready`/`/version`/
`/metrics`), so that would have been a no-op. `Client.TestConnection()`
hits `GET /api/v1/schemas` instead — genuinely authenticated, cheap,
works before a tenant is even chosen. Both of xoluman's "Test
connection" handlers switched to it; verified with a new test proving
the actual point (a server that accepts the request but rejects the
credential now correctly reports failure, which `Health()` structurally
could never detect).

**Testing this turned up something bigger, thank you for catching it:**
`apikey` auth mode sent `Authorization: Bearer <key>` since the option
existed; the server's own validator only ever accepted `X-API-Key` or
`Authorization: ApiKey <key>`. Every `apikey`-configured client was
silently unauthenticated on every request. Confirmed empirically on our
side too, not just read the fix — against a real, credential-enforcing
v0.27.0 server, the old header genuinely 401s and the new one genuinely
200s. Also found: xoluman's own test for this had the identical blind
spot yours did — a mock recording what header was sent, which would
have passed against the broken behavior just as easily. Fixed on both
sides now.

One more small thing while in the area: `authHeader()`'s own doc
comment (`pkg/client/client.go`, the `AuthAPIKey`/`WithAPIKey` doc
comments around lines 71–72 and 133) still says `"Authorization: Bearer
<key>"` for apikey mode — the code's fixed, the comment above it wasn't
updated to match. Not urgent, same five-minute-confirm category as #6
below.

## ~~6. `FieldDef.Type`'s doc comment lists `"ref"`, code never sets it that way~~ — ✅ fixed

Checked against current source while updating this letter, same as #2
above: `FieldDef.Type`'s doc comment now reads "Always set directly
from the schema's own `\"type\"` key — never `\"ref\"` or any other
xolu-specific tag; those live in `Format` instead" — exactly the
five-minute confirm that was asked for. Never blocked anything on
xoluman's side either way.

---

## ~~7. FSM definition write methods~~ — ✅ delivered in v0.26.0

(Full original ask: `docs/xolu-requests-fsm-def.md` — follow-up letter,
filed 2026-08-03, separately from the batch above.)

`CreateMachineDef`/`ReplaceMachineDef`/`DeleteMachineDef`/
`ValidateMachineDef` — confirmed directly against v0.26.0 source,
matching the request closely. Bonus beyond what was asked: `analysis`
comes back as a typed `MachineDefAnalysis` struct (reachability,
determinism, cycles, warnings) on all four, and `GetMachineDef` gained
a `ParsedAnalysis()` method decoding into the same struct.

Two behavioral notes from the delivery, worth keeping for whoever
builds xoluman's FSM editor (T-14, on its own track, not currently
active): `ReplaceMachineDef` affects future machine creation only, no
retroactive effect on already-running machines; `DeleteMachineDef` has
no reference check at all (no "count machines by definition ID"
exposed — client-side cross-referencing needed for a "warn before
deleting something in use" affordance, if wanted).

`internal/xoluext/fsmdef.go` — the hand-rolled raw-HTTP workaround
built before this shipped — has been deleted. Zero callers anywhere in
xoluman at the time it was removed.

---

## ~~8. Tenant-scoped clients wrongly prefix genuinely global endpoints~~ — ✅ already fixed, shipped before this letter arrived

(Full original detail, exact reproduction: `docs/xolu-requests-
tenant-schema.md`, filed 2026-08-04.)

Shipped in v0.26.2 — `buildURLRoot`, confirmed directly against
`pkg/client/client.go`'s own doc comment on the fix, which credits this
exact report. `GetEntitySchema`/`DefineEntitySchema`/the schema-list
call all correctly skip the tenant prefix on their own now.
`internal/xoluext.BuildSchemaClient()`, xoluman's own workaround, has
been removed — all 6 call sites reverted to the regular client, and a
new test confirms the regular client now correctly handles a schema
fetch even with a tenant configured, not just the absence of the old
workaround.

---

## ~~9. Updating an entity with an existing, undeclared-target REF field always fails~~ — ✅ delivered in v0.27.0, real cause different from our theory

(Original detail, exact reproduction, isolated variable by variable:
`docs/xolu-requests-undeclared-ref-update.md`, filed 2026-08-04 — kept
for the record; superseded by what's below.)

Our own working theory (undeclared ref target) reproduced the symptom
exactly but was wrong about the cause — thank you for checking it
directly rather than trusting it: a plain schema with no ref fields at
all reproduced the identical failure, and declaring a target did not
fix it. The real cause: `PUT`, `PATCH`, and `save` all validated a
document that already contained `id` (and for `PATCH`, `_version`) —
system fields no schema ever declares — and
`additionalProperties:false` correctly rejected them per its own spec,
on every update regardless of what was actually changed. `POST`
(create) never hit this, since a not-yet-created entity has no `id`
yet. Fixed via `stripSystemFieldsForValidation`, confirmed directly
against `pkg/server/handlers.go`/`server.go`'s three call sites, and
verified independently on our side: a correctly-shaped direct `PUT`
against the exact `examples/crm` repro now succeeds.

**A separate, genuinely distinct bug turned up once that fix was
verified — entirely ours, not yours, noting it here for completeness
rather than as an ask.** With the id/_version issue fixed, updating
`companies` *still* failed through xoluman's own web form — because
the form was submitting a bare number for `owner` (the undeclared-
target ref field), which your validator correctly rejects regardless
of the id/_version fix. This affected both create and update
identically and had nothing to do with your side. Closed on ours: the
form now asks the person for the target entity type directly when the
schema doesn't declare one, and builds the real `{type,entity,id}`
shape from that. Verified end-to-end through xoluman's actual web
form against the real CRM demo — both create and update now genuinely
succeed, confirmed persisted in the real data.

---

## Reviewed and *not* requesting

Listed so the scope above reads as reviewed, not assembled by grepping
for the first gap found in each area — unchanged since the original
filing, still accurate:

- **Bulk/multi-entity import beyond `Commit`.** `POST /api/v1/commit`
  already covers atomic multi-entity writes. xoluman's import feature
  (T-05, shipped) is built on `Create`/`Commit` as they exist.
- **OQL/Sulpher query execution.** Fully covered by `Client.OQL` and
  `Client.GraphQuery`. Nothing needed.
- **Entity CRUD, listing, pagination, search.** Fully covered.
- **Echoing the document back after a write.** Not asking for a
  change, just noting it was considered.
- **Bulk grid-editing support.** xoluman's grid editor (T-11) shipped
  on the existing per-row `Patch` calls; `Commit` wasn't needed for
  that shape of write after all.

---

## Status: everything in this letter is closed

All nine items — the original six, the FSM-def follow-up, and both
bugs found along the way — are resolved as of xolu v0.27.0. Synced
xoluman against v0.27.1 as well (2026-08-04): reviewed directly, the
only change in that release is `CreateMachineDef`/`ReplaceMachineDef`
client-side validation for Seam AMS's own request (T-162) — confirmed
xoluman doesn't call either method yet (T-14, the FSM def module, isn't
built), so this was a clean version bump with no code changes needed.

Nothing outstanding needs a response. The two minor items noted in
passing above — the stale `apikey` doc comment (#5) and this letter's
own note in #6 — are both five-minute-confirm category, not urgent,
not blocking anything. Whatever turns up next, xoluman's own tracking
(`docs/TRACKING.md`) is where it'll start, same as always.
