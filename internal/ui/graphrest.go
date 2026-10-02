// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"context"
	"fmt"

	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/connstore"
	"github.com/ha1tch/xoluman/internal/xoluext"
)

// graphRunner is the swappable boundary between the graph viewer's HTTP
// handler and whatever actually executes a graph query. Two
// implementations exist:
//
//   - restEmbedGraphRunner (this file, currently the only one wired
//     up): builds {nodes, edges} from plain REST calls, walking REF
//     fields directly rather than running any query language at all.
//   - The dormant Sulpher-based path this replaced (query.go's own
//     Graph handler, still present in version control): MATCH ...
//     RETURN against xolu's /graph/query endpoint, blocked end to end
//     by XM-9 (whole-node RETURN failing against schema-adapted
//     entities — xoluman-xolu-consolidated-report-log.md) for every
//     entity this session's own CRM example uses, since all of them
//     have a schema registered.
//
// Swapping back once XM-9 is resolved is a matter of changing which
// implementation the Graph handler constructs, not a change to the
// handler itself, the JS graph viewer, or the {nodes, edges} contract
// either side of this depends on — that contract (graphNode, graphEdge,
// graphData in query.go) predates this file and is unchanged by it.
type graphRunner interface {
	Run(ctx context.Context, c *xclient.Client, conn connstore.Connection, spec restGraphSpec) (graphData, error)
}

// restGraphSpec is what the REST-backed runner needs to know: which
// entity type to start from, how many hops of REF relationships to
// follow, and how many top-level entities to fetch.
type restGraphSpec struct {
	EntityType string
	EmbedDepth int
	Limit      int
}

// maxGraphEmbedDepth bounds how many REF hops restEmbedGraphRunner will
// ever follow — a runaway depth against a densely-connected schema
// could pull in a large fraction of the whole dataset, one HTTP
// request per newly-discovered node (see the design note on Run below
// for why this is client-side, request-per-hop rather than a single
// server-side embed_depth fetch). Chosen well above what any of this
// session's own CRM example queries need (two hops covers every saved
// graph query it has) while still being a real, enforced ceiling
// rather than trusting the caller.
const maxGraphEmbedDepth = 4

// maxGraphTopLevelEntities bounds the top-level fetch (the Limit sent
// to ListWithEmbed) for the same reason — a graph viewer rendering
// hundreds of nodes at once stops being a graph anyone can read.
const maxGraphTopLevelEntities = 100

// maxGraphTotalFetches is a hard ceiling on the total number of HTTP
// requests one graph query will make (the top-level list call plus one
// follow-up Get per newly-discovered node) — the real backstop against
// a densely-connected schema at maxGraphEmbedDepth fanning out into an
// impractical number of requests, independent of how high the other
// two limits are individually.
const maxGraphTotalFetches = 300

type restEmbedGraphRunner struct{}

// Run builds {nodes, edges} entirely from plain REST calls — no query
// language, no Sulpher, no OQL.
//
// Design note, the reason for what might look like an odd choice
// (never using the server's own embed_depth for anything beyond 0):
// tried the more obvious design first — one ListWithEmbed call at the
// requested depth, letting the server do the hydration — and it
// silently produced edges with an empty target type whenever a REF
// field's schema declared no explicit target (confirmed directly
// against this session's own real CRM data: an edge read
// "deals:1 -[owner]-> :4", not "deals:1 -[owner]-> users:4"). Root
// cause: a server-hydrated REF value never self-identifies its own
// entity type (confirmed directly — no "type"/"entity" marker survives
// embedding), and most of this session's own CRM schema declares REF
// fields with no target extension at all — the same real, common case
// the grid editor's own reconstructRefValues fallback exists for. A
// bare pointer always self-identifies its type via its own "entity"
// key; a hydrated value only does if the schema happened to declare
// one. So: always fetch bare (embed_depth=0 explicitly, not omitted —
// GetWithEmbed's own doc comment on why "explicit 0" and "omitted"
// aren't the same request), and do the hydration ourselves, one hop at
// a time, using each hop's own bare pointer to know reliably what to
// fetch next.
func (restEmbedGraphRunner) Run(ctx context.Context, c *xclient.Client, conn connstore.Connection, spec restGraphSpec) (graphData, error) {
	data := graphData{Nodes: []graphNode{}, Edges: []graphEdge{}}
	if spec.EntityType == "" {
		return data, fmt.Errorf("entity type is required")
	}
	depth := spec.EmbedDepth
	if depth < 0 {
		depth = 0
	}
	if depth > maxGraphEmbedDepth {
		depth = maxGraphEmbedDepth
	}
	limit := spec.Limit
	if limit <= 0 || limit > maxGraphTopLevelEntities {
		limit = maxGraphTopLevelEntities
	}

	result, err := xoluext.ListWithEmbed(ctx, c, conn, spec.EntityType, 0, &xclient.ListParams{Limit: limit})
	if err != nil {
		return data, err
	}

	w := &graphWalker{
		ctx: ctx, c: c, conn: conn,
		resolver:       newGraphRefResolver(ctx, c),
		data:           &data,
		seenNodes:      map[string]bool{},
		collapsedIndex: map[string]int{},
		seenEdges:      map[string]bool{},
		fetches:        1, // the ListWithEmbed call above already counts against the ceiling
	}
	for _, e := range result.Entities {
		w.walkEntity(spec.EntityType, e.ID, e.Data, depth)
	}
	return data, nil
}

// Expand fetches a single, specific entity in full — one previously
// shown collapsed, its own REF fields never walked — and walks those
// fields up to depth hops, exactly the way Run does for a fresh
// entity-type list, just seeded from one already-known node instead.
// The answer to clicking a collapsed node and asking "show me what
// this points at": xoluman's own graph viewer offers this as an
// explicit action rather than eagerly expanding every collapsed node
// up front, which is what maxGraphEmbedDepth/maxGraphTotalFetches
// exist to bound against in the first place.
func (restEmbedGraphRunner) Expand(ctx context.Context, c *xclient.Client, conn connstore.Connection, entityType string, id int64, depth int) (graphData, error) {
	data := graphData{Nodes: []graphNode{}, Edges: []graphEdge{}}
	if entityType == "" || id <= 0 {
		return data, fmt.Errorf("entity type and a positive id are required")
	}
	if depth < 0 {
		depth = 0
	}
	if depth > maxGraphEmbedDepth {
		depth = maxGraphEmbedDepth
	}

	entity, err := xoluext.GetWithEmbed(ctx, c, conn, entityType, id, 0)
	if err != nil {
		return data, err
	}

	w := &graphWalker{
		ctx: ctx, c: c, conn: conn,
		resolver:       newGraphRefResolver(ctx, c),
		data:           &data,
		seenNodes:      map[string]bool{},
		collapsedIndex: map[string]int{},
		seenEdges:      map[string]bool{},
		fetches:        1,
	}
	w.walkEntity(entityType, id, entity.Data, depth)
	return data, nil
}

// graphRefResolver caches schema-derived REF field information per
// entity type for the lifetime of one graph query — a walk over even a
// modest result set can revisit the same entity type many times (every
// "deals" node needs the same "deals" ref-field map), and each lookup
// is itself a schema fetch plus a fieldmeta list call, neither of
// which change mid-walk.
type graphRefResolver struct {
	ctx   context.Context
	c     *xclient.Client
	cache map[string][]string // entityType -> REF-formatted field names
}

func newGraphRefResolver(ctx context.Context, c *xclient.Client) *graphRefResolver {
	return &graphRefResolver{ctx: ctx, c: c, cache: map[string][]string{}}
}

// refFieldNamesFor returns every field name on entityType that's
// REF-formatted. The target entity type is deliberately not resolved
// here at all (unlike the grid editor's own refTargetsForSubmission) —
// with every REF value fetched bare (see Run's own design note), the
// target type comes reliably from each value's own "entity" key, and
// schema/remembered-target lookups are exactly the mechanism that
// silently produced an unknown target when they were relied on here.
func (r *graphRefResolver) refFieldNamesFor(entityType string) []string {
	if cached, ok := r.cache[entityType]; ok {
		return cached
	}
	schema, err := r.c.GetEntitySchema(r.ctx, entityType)
	var names []string
	if err == nil && schema != nil {
		for _, f := range schema.Fields {
			if f.Format == "ref" || f.Type == "ref" {
				names = append(names, f.Name)
			}
		}
	}
	r.cache[entityType] = names
	return names
}

// graphWalker builds {nodes, edges} from fetched entity documents,
// fetching further hops itself (via xoluext.GetWithEmbed, always
// bare — see Run's own design note) as it discovers new REF targets.
type graphWalker struct {
	ctx  context.Context
	c    *xclient.Client
	conn connstore.Connection

	resolver  *graphRefResolver
	data      *graphData
	seenNodes map[string]bool
	// collapsedIndex tracks nodes currently present only as a
	// collapsed stub (Collapsed: true, minimal Data), keyed the same
	// way as seenNodes, valued by that node's own index into
	// data.Nodes — lets walkEntity find and upgrade a stub in place
	// (rather than appending a duplicate) the moment real data for it
	// arrives, whether that's later in the same walk (another path
	// reaches it with hop budget to spare) or via a later, separate
	// Expand call. Removed from this map the moment a node is
	// upgraded — collapsedIndex and seenNodes together fully describe
	// a node's state: present+collapsed, present+full, or absent.
	collapsedIndex map[string]int
	seenEdges      map[string]bool
	fetches        int
}

// walkEntity adds entityType:id as a node — fresh, or upgrading an
// existing collapsed stub for the same key in place — and walks every
// REF field on data, adding an edge for each. remainingHops is the hop
// budget left: when it runs out (or the request-count ceiling does)
// for a given REF field's target, the edge is still recorded and the
// target still gets a node — collapsed, not fetched, not walked any
// further — rather than a dangling edge to a node that was never
// shown at all. A fetch failure (unreachable target, permissions edge
// case, transient error) degrades the same way: the edge and a
// collapsed stub stay, the walk itself doesn't fail.
func (w *graphWalker) walkEntity(entityType string, id int64, data map[string]any, remainingHops int) {
	key := entityType + ":" + fmt.Sprint(id)
	if idx, wasCollapsed := w.collapsedIndex[key]; wasCollapsed {
		w.data.Nodes[idx] = graphNode{ID: key, Type: entityType, Data: data}
		delete(w.collapsedIndex, key)
	} else if w.seenNodes[key] {
		return // already fully present, nothing left to do
	} else {
		w.seenNodes[key] = true
		w.data.Nodes = append(w.data.Nodes, graphNode{ID: key, Type: entityType, Data: data})
	}

	for _, field := range w.resolver.refFieldNamesFor(entityType) {
		raw, present := data[field]
		if !present || raw == nil {
			continue
		}
		targetType, targetID, ok := decodeBareRef(raw)
		if !ok {
			continue // couldn't determine even a target ID -- nothing to draw
		}
		w.addEdge(entityType, id, field, targetType, targetID)

		targetKey := targetType + ":" + fmt.Sprint(targetID)
		if remainingHops <= 0 {
			w.addCollapsedStubIfMissing(targetType, targetID, targetKey)
			continue
		}
		_, targetIsCollapsed := w.collapsedIndex[targetKey]
		if w.seenNodes[targetKey] && !targetIsCollapsed {
			continue // already fully fetched via another path -- nothing more to do
		}
		if w.fetches >= maxGraphTotalFetches {
			w.addCollapsedStubIfMissing(targetType, targetID, targetKey)
			continue // hard request ceiling reached -- stays collapsed rather than exceeding it
		}
		w.fetches++
		targetEntity, err := xoluext.GetWithEmbed(w.ctx, w.c, w.conn, targetType, targetID, 0)
		if err != nil {
			w.addCollapsedStubIfMissing(targetType, targetID, targetKey)
			continue
		}
		w.walkEntity(targetType, targetID, targetEntity.Data, remainingHops-1)
	}
}

// addCollapsedStubIfMissing records targetType:targetID as a
// collapsed node if it isn't present in any form yet. A no-op if the
// node already exists, collapsed or not — never downgrades a node
// that's already fully fetched, and never adds a second stub for one
// already stubbed (multiple edges converging on the same
// not-yet-fetched target are common and expected, not something each
// one should re-stub).
func (w *graphWalker) addCollapsedStubIfMissing(targetType string, targetID int64, targetKey string) {
	if w.seenNodes[targetKey] {
		return
	}
	w.seenNodes[targetKey] = true
	w.collapsedIndex[targetKey] = len(w.data.Nodes)
	w.data.Nodes = append(w.data.Nodes, graphNode{
		ID: targetKey, Type: targetType, Collapsed: true,
		Data: map[string]any{"id": targetID},
	})
}

// decodeBareRef extracts (targetType, targetID) from a REF field's raw
// value. With every fetch in this walk requesting embed_depth=0
// explicitly (see Run's own design note), this is always the bare
// {"type":"REF","entity":"<target>","id":N} shape in practice — the
// hydrated-document branch a much earlier version of this function had
// is gone, along with the unknown-target-type failure mode it caused.
func decodeBareRef(raw any) (targetType string, targetID int64, ok bool) {
	m, isMap := raw.(map[string]any)
	if !isMap {
		return "", 0, false
	}
	entity, hasEntity := m["entity"].(string)
	id, idOK := m["id"].(float64)
	if !hasEntity || !idOK || entity == "" {
		return "", 0, false
	}
	return entity, int64(id), true
}

func (w *graphWalker) addEdge(fromType string, fromID int64, rel string, toType string, toID int64) {
	from := fromType + ":" + fmt.Sprint(fromID)
	to := toType + ":" + fmt.Sprint(toID)
	key := from + "|" + rel + "|" + to
	if w.seenEdges[key] {
		return
	}
	w.seenEdges[key] = true
	w.data.Edges = append(w.data.Edges, graphEdge{From: from, To: to, Rel: rel})
}
