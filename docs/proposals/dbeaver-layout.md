Updated: 2026-08-04

# Proposal: A DBeaver-style layout for xoluman

## What's being asked for

DBeaver's layout has three parts that stay on screen together:

- A **left sidebar tree** — connections, expandable down into schemas/
  tables — always visible, always expanded to wherever you left it.
- A **tabbed main workspace** — opening a table, a query, another
  table, all stay open as separate tabs you switch between, not a
  stack you navigate back and forth through.
- A **bottom results/log panel** attached to whichever tab is active.

None of this is unique to DBeaver — it's the standard shape for
basically every desktop-grade data tool (pgAdmin, DataGrip, Azure Data
Studio, TablePlus). The appeal is real: you can have three tables and a
query open at once, jump between them instantly, and the tree never
disappears while you work.

## What xoluman is today, and why this isn't a small change

xoluman is a classic server-rendered, multi-page app. Every URL is a
full page; htmx is used narrowly — a modal here, a status swap there —
never to keep a persistent shell alive across navigation. Clicking
"widgets" then "gadgets" then back to "widgets" is three full page
loads. There is no concept of "still-open tabs" anywhere, because
nothing persists across a navigation at all — the browser's own history
*is* xoluman's navigation model.

That's not an accident or an oversight; it's most of why the codebase
has stayed simple enough to build this much of it, this fast, without
a client-side state layer to reason about. A DBeaver-style layout
requires exactly the thing that simplicity trades away: **state that
survives navigation** — which tabs are open, in what order, scroll
position, an unsaved edit sitting in a tab you're not currently
looking at. A full page reload destroys all of that. There is no way
to get real persistent tabs without introducing *some* client-side
state management, whether that's a full SPA framework or a more
minimal approach.

## Two things worth separating, because their cost is very different

**The sidebar tree** (connection → entity types, and now blobs/query
too) is the *lower-cost, higher-value* half of the DBeaver feel. It
doesn't strictly need real SPA state — htmx already has a pattern for
exactly this: `hx-boost` on the main content links, with the sidebar
living *outside* the swapped region so it never re-renders on
navigation. The URL still changes, the browser back button still
works, but the sidebar stops flickering and stays exactly where you
left it (scroll position, expanded state) because it's never touched.
This is a real, working htmx pattern, not a hack — it's what htmx's
boosting feature exists for.

**Real tabs** are the *expensive* half. "Three tables open at once,
switch between them instantly, one has an unsaved edit" cannot be done
with page loads, boosted or not — every tab's content and edit state
has to live in the browser, independent of what URL is currently
showing. That means either a real client-side router + state store
(the SPA path — a genuine architectural rewrite, not a feature added
on top of the current app), or an approximation that keeps multiple
iframes/hidden panels alive at once and swaps their visibility (workable,
but a real amount of new machinery, and iframes bring their own
awkwardness — no shared scroll/keyboard context with the parent page,
harder styling coordination).

## Recommendation: two phases, the second one optional

### Phase 1 — persistent sidebar tree (genuinely worth doing)

- A left sidebar, always visible on every `/connections/{name}/...`
  page: the connection name at the top, then entity types (from
  `ListEntities`, reusing exactly what T-17 already built) and blob
  folders as expandable tree nodes.
- `hx-boost` on the sidebar's own links and the main content area, with
  the sidebar rendered *outside* whatever `hx-target` the boosted
  navigation swaps — so it survives every click within that
  connection untouched.
- Cost: real, but bounded — a new sidebar component, a small nav-shell
  change in `layout.go`, and confirming `hx-boost` behaves correctly
  with the existing modal/htmx pieces already in place. No new
  state-management concept introduced; still a multi-page app
  underneath.
- This gets most of what people actually value about DBeaver's layout
  — the tree that's just *there*, that you don't lose your place in —
  without touching the app's fundamental architecture.

### Phase 2 — real tabs (a genuine rewrite, only worth it if Phase 1
isn't enough)

- Would mean picking a client-side approach — most realistically, a
  Lit-shell "workspace" component (matching the pattern already used
  for the grid and query editors) that owns a set of open tabs
  client-side, fetches each tab's content via `fetch()` instead of a
  real navigation, and renders whichever tab is active. The server-side
  handlers barely change; what changes is that the *browser* decides
  when to call them, not full-page navigation.
- Real, non-trivial new problems this introduces: what happens to a
  tab's state if you refresh the browser (currently nothing to lose,
  suddenly everything is); how unsaved-edit warnings work when closing
  a tab versus leaving the page entirely; how the URL and the active
  tab agree with each other, since with real tabs the URL stops being
  the single source of truth for "what's on screen" the way it is
  today.
- This is a multi-week architectural project, not a feature. Not
  proposing to start it now — recorded here so the trade-off is
  visible, not so it's decided today.

## What I'd suggest

Phase 1 only, for now. It's a real, scoped, finishable piece of work
that captures the part of this request most people actually mean when
they say "I want it to feel like DBeaver" — a tree that stays put.
Phase 2 is a legitimate thing to want eventually, but it's a different
kind of project than everything else in this codebase so far, and
should be a deliberate decision with its own design pass, not
something backed into as an extension of Phase 1.
