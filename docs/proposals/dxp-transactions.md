Updated: 2026-08-04

# Proposal: DXP transaction support in the query editor

## What DXP actually is, confirmed against the real client

Checked directly against `xolu/pkg/client/dxp.go` and `types_dxp.go`,
not assumed from the name:

- **`DxpDef`** — a registered, reusable template: a `Name`, a
  coordination `Pattern`, a list of `Participants` (each targeting one
  xolu primitive — `bal`, `cal`, `fsm`, `entity`, `ts` — with an `Op`
  and `Params`), a `PhaseTTL`, and an optional `BindingsSchema`. A
  participant's `Params` can contain `{"$ref": "<binding name>"}`
  placeholders — this is where "parameter slots" live.
- **`DxpTxn`** — one invocation of a def: `DefID` plus `Bindings`, a
  flat map filling in whatever `$ref` names the def's participants
  reference. Dispatches synchronously — `POST /dxp/txn` returns an
  already-terminal instance (`committed`, `released`, or `expired`,
  with a `Reason` on anything short of `committed`), not something to
  poll.

This matches the framing that prompted this proposal exactly: a DXP
transaction is closer to calling a stored procedure with named
parameters than running a query. There's no query *language* — the
"query" is choosing a def and filling in its bindings.

## Why this doesn't fit as a fourth CodeMirror tab

OQL, Sulpher, and REST all share one shape: a person writes something,
the editor highlights it, it gets sent as-is. DXP has a structurally
different flow:

1. **Choose an existing def.** Defs are registered ahead of time
   (`DxpDefCreate`), not typed fresh per invocation — closer to how a
   schema is registered once and then used by many entity writes.
   There's nothing to "write" here, only to *pick*.
2. **See what it needs.** A chosen def's participants reference some
   set of binding names via `$ref` — these need to be extracted from
   the def's own spec (`DxpDefGet` returns the full `Spec`, not just
   the summary `DxpDefList` gives) before anyone can know what to fill
   in. `BindingsSchema`, when present, describes the *shape* those
   bindings should take.
3. **Fill in the slots.** One input per distinct binding name the
   participants reference — genuinely a small, dynamically-generated
   form, not a code editor at all.
4. **Run it, see the outcome.** `committed`/`released`/`expired` plus
   `Reason` — a different kind of result than a row set or a raw HTTP
   response.

Bolting this onto the CodeMirror-based tab structure would mean either
distorting the UI to pretend a form is a code editor, or growing the
existing component into something aware of two entirely different
interaction models. Neither is good — this earns its own space, not a
fourth `MODES` entry.

## What building it would actually take

- **Def picker**: a dropdown fed by `DxpDefList` (cheap — summaries
  only). Selecting one triggers `DxpDefGet` for the full spec.
- **Binding-name extraction**: walk every participant's `Params`,
  collecting every `{"$ref": "..."}` value found (including nested —
  `Params` is `map[string]interface{}`, so a `$ref` could sit inside a
  nested object or array, not just at the top level of a param). No
  existing code in this codebase does this kind of walk; it would be
  new.
- **Form generation**: one input per collected binding name. If
  `BindingsSchema` is present, its JSON Schema properties give real
  type/required hints per binding — reusable in spirit with
  `internal/formengine`'s existing schema-to-form logic, but that
  package is built around `client.FieldDef` (xolu's *entity* field
  shape), not a raw JSON Schema object; adapting it or building a
  parallel path is real work, not a trivial reuse.
- **Execution + result display**: `DxpTxnCreate`, then render
  `Status`/`Reason`/`CommittedThrough` clearly — a rejected or partial
  outcome needs to read as informative, not as an error page.
- **"Saved DXP invocations"**: a saved (def ID, bindings) pair, not a
  saved query string — the same `xoluman_saved_query`-style pattern
  the other three modes now use, but the payload shape is different
  enough that it's a genuinely separate saved-item type, not a
  variant of the one just built.

None of this is *hard*, but it's a real, multi-piece feature — binding
extraction and dynamic form generation in particular are new problems
this codebase hasn't solved before, not a reuse of something already
built.

## Recommendation

Worth building — DXP is a first-class xolu primitive and the query
editor is exactly the place someone would expect to reach it — but as
its own scoped pass, not folded into the query-editor work just
finished. Suggested shape for that pass:

1. A separate `/connections/{name}/query/dxp` view (or a genuinely
   distinct tab that swaps the whole editor area for a form, not a
   CodeMirror instance) rather than a fourth `MODES` entry.
2. Def picker → spec fetch → binding extraction → generated form →
   run, in that order, each step real and tested before the next.
3. Saved DXP invocations as a deliberately separate saved-item concept
   from this pass's saved-query feature, not retrofitted onto it.

Not started as part of this pass. Flagging here so the decision is
explicit rather than DXP quietly falling off the list.
