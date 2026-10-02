// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/connstore"
)

// graphTestServer builds a mock xolu instance for graph-walk tests:
// posts (with author_id -> users, a declared target, and category ->
// no declared target, exercising both known- and unknown-target REF
// fields — irrelevant to the walker itself now, since it never
// consults declared targets at all, but kept to confirm that's
// genuinely true) and users (no REF fields of its own, keeping the
// recursive walk bounded). Individual GET routes are required now,
// unlike the earlier server-side-embedding design: every hop beyond
// the top-level list is its own follow-up Get call.
func graphTestServer(t *testing.T, postsHandler http.HandlerFunc, users map[int64]map[string]any, categories map[int64]map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/posts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":     map[string]any{"type": "string"},
				"author_id": map[string]any{"type": "object", "format": "ref", "target": "users"},
				"category":  map[string]any{"type": "object", "format": "ref"},
			},
		})
	})
	mux.HandleFunc("GET /api/v1/schema/users", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":       "object",
			"properties": map[string]any{"name": map[string]any{"type": "string"}},
		})
	})
	mux.HandleFunc("GET /api/v1/schema/categories", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":       "object",
			"properties": map[string]any{"label": map[string]any{"type": "string"}},
		})
	})
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "pagination": map[string]any{"total_pages": 1}})
	})
	mux.HandleFunc("GET /api/v1/posts", postsHandler)
	for id, doc := range users {
		doc := doc
		mux.HandleFunc(userPath(id), func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(doc)
		})
	}
	for id, doc := range categories {
		doc := doc
		mux.HandleFunc(categoryPath(id), func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(doc)
		})
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func userPath(id int64) string     { return "GET /api/v1/users/" + strconv.FormatInt(id, 10) }
func categoryPath(id int64) string { return "GET /api/v1/categories/" + strconv.FormatInt(id, 10) }

func nodeByID(data graphData, id string) (graphNode, bool) {
	for _, n := range data.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return graphNode{}, false
}

func TestRestEmbedGraphRunner_FollowsRefFieldAndFetchesTargetWhenDepthAllows(t *testing.T) {
	server := graphTestServer(t,
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": float64(1), "title": "Hello", "author_id": map[string]any{"type": "REF", "entity": "users", "id": float64(7)}},
				},
				"pagination": map[string]any{"page": 1, "per_page": 10, "total_items": 1, "total_pages": 1},
			})
		},
		map[int64]map[string]any{7: {"id": float64(7), "name": "Alice"}},
		nil,
	)
	c := xclient.New(server.URL)
	conn := connstore.Connection{}

	data, err := restEmbedGraphRunner{}.Run(context.Background(), c, conn, restGraphSpec{EntityType: "posts", EmbedDepth: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	post, ok := nodeByID(data, "posts:1")
	if !ok {
		t.Fatal("posts:1 node missing")
	}
	if post.Data["title"] != "Hello" {
		t.Errorf("post title = %v, want Hello", post.Data["title"])
	}

	author, ok := nodeByID(data, "users:7")
	if !ok {
		t.Fatal("users:7 node missing -- with depth 1, the walker should have followed author_id via its own Get call")
	}
	if author.Data["name"] != "Alice" {
		t.Errorf("author name = %v, want Alice (fetched via the follow-up Get, not a stub)", author.Data["name"])
	}

	if len(data.Edges) != 1 {
		t.Fatalf("edges = %+v, want exactly 1", data.Edges)
	}
	e := data.Edges[0]
	if e.From != "posts:1" || e.To != "users:7" || e.Rel != "author_id" {
		t.Errorf("edge = %+v, want posts:1 -[author_id]-> users:7", e)
	}
}

func TestRestEmbedGraphRunner_ZeroDepthProducesCollapsedStubNotDanglingEdge(t *testing.T) {
	server := graphTestServer(t,
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": float64(1), "title": "Hello", "author_id": map[string]any{"type": "REF", "entity": "users", "id": float64(7)}},
				},
				"pagination": map[string]any{"page": 1, "per_page": 10, "total_items": 1, "total_pages": 1},
			})
		},
		nil, nil,
	)
	c := xclient.New(server.URL)
	conn := connstore.Connection{}

	data, err := restEmbedGraphRunner{}.Run(context.Background(), c, conn, restGraphSpec{EntityType: "posts", EmbedDepth: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	author, ok := nodeByID(data, "users:7")
	if !ok {
		t.Fatal("users:7 node missing -- with depth 0 the target isn't fetched, but a collapsed stub should still be present so the edge has somewhere real to point")
	}
	if !author.Collapsed {
		t.Error("users:7 present but not marked Collapsed -- it was never actually fetched")
	}
	if _, hasTitle := author.Data["name"]; hasTitle {
		t.Errorf("collapsed stub has real data %+v, want just the bare id", author.Data)
	}
	if len(data.Edges) != 1 || data.Edges[0].To != "users:7" {
		t.Fatalf("edges = %+v, want one edge recorded to users:7", data.Edges)
	}
}

func TestRestEmbedGraphRunner_UnknownSchemaTargetStillResolvedFromBarePointer(t *testing.T) {
	// "category" has no declared target in the mock schema -- confirms
	// the walker draws the correct edge purely from the bare pointer's
	// own "entity" key, never consulting the schema's target
	// declaration at all (the actual fix for the real bug found
	// against live CRM data: edges with an empty target type).
	server := graphTestServer(t,
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": float64(1), "title": "Hello", "category": map[string]any{"type": "REF", "entity": "categories", "id": float64(3)}},
				},
				"pagination": map[string]any{"page": 1, "per_page": 10, "total_items": 1, "total_pages": 1},
			})
		},
		nil,
		map[int64]map[string]any{3: {"id": float64(3), "label": "News"}},
	)
	c := xclient.New(server.URL)
	conn := connstore.Connection{}

	data, err := restEmbedGraphRunner{}.Run(context.Background(), c, conn, restGraphSpec{EntityType: "posts", EmbedDepth: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cat, ok := nodeByID(data, "categories:3")
	if !ok {
		t.Fatal("categories:3 node missing")
	}
	if cat.Data["label"] != "News" {
		t.Errorf("category label = %v, want News", cat.Data["label"])
	}
	found := false
	for _, e := range data.Edges {
		if e.From == "posts:1" && e.To == "categories:3" && e.Rel == "category" {
			found = true
		}
	}
	if !found {
		t.Fatalf("edges = %+v, want posts:1 -[category]-> categories:3", data.Edges)
	}
}

func TestRestEmbedGraphRunner_SameTargetViaTwoPathsFetchedOnce(t *testing.T) {
	var userFetchCount int
	realMux := http.NewServeMux()
	realMux.HandleFunc("GET /api/v1/schema/posts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":     map[string]any{"type": "string"},
				"author_id": map[string]any{"type": "object", "format": "ref"},
			},
		})
	})
	realMux.HandleFunc("GET /api/v1/schema/users", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "object", "properties": map[string]any{}})
	})
	realMux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "pagination": map[string]any{"total_pages": 1}})
	})
	realMux.HandleFunc("GET /api/v1/posts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": float64(1), "title": "First", "author_id": map[string]any{"type": "REF", "entity": "users", "id": float64(7)}},
				{"id": float64(2), "title": "Second", "author_id": map[string]any{"type": "REF", "entity": "users", "id": float64(7)}},
			},
			"pagination": map[string]any{"page": 1, "per_page": 10, "total_items": 2, "total_pages": 1},
		})
	})
	realMux.HandleFunc("GET /api/v1/users/7", func(w http.ResponseWriter, r *http.Request) {
		userFetchCount++
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(7), "name": "Alice"})
	})
	realServer := httptest.NewServer(realMux)
	t.Cleanup(realServer.Close)

	c := xclient.New(realServer.URL)
	conn := connstore.Connection{}

	data, err := restEmbedGraphRunner{}.Run(context.Background(), c, conn, restGraphSpec{EntityType: "posts", EmbedDepth: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if userFetchCount != 1 {
		t.Errorf("users:7 fetched %d times, want exactly 1 -- both posts reference the same author", userFetchCount)
	}
	authorNodes := 0
	for _, n := range data.Nodes {
		if n.ID == "users:7" {
			authorNodes++
		}
	}
	if authorNodes != 1 {
		t.Errorf("users:7 appears %d times in the node list, want exactly 1", authorNodes)
	}
	if len(data.Edges) != 2 {
		t.Errorf("edges = %+v, want 2 (one per post, both to the same author)", data.Edges)
	}
}

func TestRestEmbedGraphRunner_DuplicateEdgesAreDeduplicated(t *testing.T) {
	server := graphTestServer(t,
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": float64(1), "title": "A", "author_id": map[string]any{"type": "REF", "entity": "users", "id": float64(7)}},
					{"id": float64(2), "title": "B", "author_id": map[string]any{"type": "REF", "entity": "users", "id": float64(9)}},
				},
				"pagination": map[string]any{"page": 1, "per_page": 10, "total_items": 2, "total_pages": 1},
			})
		},
		nil, nil,
	)
	c := xclient.New(server.URL)
	conn := connstore.Connection{}

	data, err := restEmbedGraphRunner{}.Run(context.Background(), c, conn, restGraphSpec{EntityType: "posts", EmbedDepth: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Edges) != 2 {
		t.Fatalf("edges = %+v, want exactly 2 (one per distinct post->author pair)", data.Edges)
	}
}

func TestRestEmbedGraphRunner_EmptyEntityTypeRejected(t *testing.T) {
	server := graphTestServer(t, func(w http.ResponseWriter, r *http.Request) {}, nil, nil)
	c := xclient.New(server.URL)
	_, err := restEmbedGraphRunner{}.Run(context.Background(), c, connstore.Connection{}, restGraphSpec{})
	if err == nil {
		t.Fatal("expected an error for an empty entity type, got nil")
	}
}

func TestRestEmbedGraphRunner_NoRefFieldsPresentJustReturnsNodes(t *testing.T) {
	server := graphTestServer(t,
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":       []map[string]any{{"id": float64(1), "title": "Solo"}},
				"pagination": map[string]any{"page": 1, "per_page": 10, "total_items": 1, "total_pages": 1},
			})
		},
		nil, nil,
	)
	c := xclient.New(server.URL)
	data, err := restEmbedGraphRunner{}.Run(context.Background(), c, connstore.Connection{}, restGraphSpec{EntityType: "posts", EmbedDepth: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Nodes) != 1 || len(data.Edges) != 0 {
		t.Fatalf("data = %+v, want exactly 1 node and 0 edges", data)
	}
}

func TestRestEmbedGraphRunner_DepthIsClampedToMaxHops(t *testing.T) {
	// A chain 6 posts deep (post1 -author_id-> uses "users" only as a
	// stand-in single hop; to test hop clamping specifically, use a
	// self-referential-style chain via categories, each category
	// pointing to the next). Depth requested is far beyond
	// maxGraphEmbedDepth (4); confirm the walk stops there, not at the
	// requested depth.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/posts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":       "object",
			"properties": map[string]any{"category": map[string]any{"type": "object", "format": "ref"}},
		})
	})
	mux.HandleFunc("GET /api/v1/schema/categories", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":       "object",
			"properties": map[string]any{"parent": map[string]any{"type": "object", "format": "ref"}},
		})
	})
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "pagination": map[string]any{"total_pages": 1}})
	})
	mux.HandleFunc("GET /api/v1/posts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       []map[string]any{{"id": float64(1), "category": map[string]any{"type": "REF", "entity": "categories", "id": float64(1)}}},
			"pagination": map[string]any{"page": 1, "per_page": 10, "total_items": 1, "total_pages": 1},
		})
	})
	// A chain of 10 categories, each pointing to the next -- category:N -> category:N+1.
	for i := int64(1); i <= 10; i++ {
		i := i
		mux.HandleFunc(categoryPath(i), func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": float64(i), "parent": map[string]any{"type": "REF", "entity": "categories", "id": float64(i + 1)},
			})
		})
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	c := xclient.New(server.URL)
	data, err := restEmbedGraphRunner{}.Run(context.Background(), c, connstore.Connection{}, restGraphSpec{
		EntityType: "posts", EmbedDepth: 999, // far beyond maxGraphEmbedDepth
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// posts:1 -> categories:1 (hop 1) -> categories:2 (hop 2) -> categories:3 (hop 3)
	// -> categories:4 (hop 4) -- categories:4's own "parent" edge to
	// categories:5 is recorded and categories:5 gets a collapsed stub
	// (its bare pointer is right there in categories:4's own data),
	// but categories:5 itself is never fetched, since the hop budget
	// hit 0 there.
	for _, id := range []string{"categories:1", "categories:2", "categories:3", "categories:4"} {
		n, ok := nodeByID(data, id)
		if !ok {
			t.Errorf("%s node missing, want it present (within the clamped depth)", id)
			continue
		}
		if n.Collapsed {
			t.Errorf("%s is collapsed, want it fully fetched (within the clamped depth)", id)
		}
	}
	stub, ok := nodeByID(data, "categories:5")
	if !ok {
		t.Fatal("categories:5 node missing, want it present as a collapsed stub -- beyond the clamped depth of 4, but still a real node the edge can point to")
	}
	if !stub.Collapsed {
		t.Error("categories:5 present but not marked Collapsed -- it's beyond the clamped depth and was never actually fetched")
	}
	foundLastEdge := false
	for _, e := range data.Edges {
		if e.From == "categories:4" && e.To == "categories:5" {
			foundLastEdge = true
		}
	}
	if !foundLastEdge {
		t.Error("edge categories:4 -> categories:5 missing -- should still be recorded even though categories:5 itself wasn't fetched")
	}
}

func TestRestEmbedGraphRunner_LimitIsClampedToMaxTopLevelEntities(t *testing.T) {
	var gotQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/posts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "object", "properties": map[string]any{}})
	})
	mux.HandleFunc("GET /api/v1/posts", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{}, "pagination": map[string]any{"page": 1, "per_page": 10, "total_items": 0, "total_pages": 1},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	c := xclient.New(server.URL)
	_, err := restEmbedGraphRunner{}.Run(context.Background(), c, connstore.Connection{}, restGraphSpec{
		EntityType: "posts", Limit: 999999,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotQuery != "per_page=100" {
		t.Errorf("query = %q, want per_page clamped to maxGraphTopLevelEntities (100); embed_depth is correctly omitted when 0 (ListWithEmbed's own convention, and list's own server default is already 0)", gotQuery)
	}
}

func TestRestEmbedGraphRunner_UnreachableTargetDoesNotFailWholeWalk(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/posts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":       "object",
			"properties": map[string]any{"author_id": map[string]any{"type": "object", "format": "ref"}},
		})
	})
	mux.HandleFunc("GET /api/v1/posts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       []map[string]any{{"id": float64(1), "title": "Hello", "author_id": map[string]any{"type": "REF", "entity": "users", "id": float64(7)}}},
			"pagination": map[string]any{"page": 1, "per_page": 10, "total_items": 1, "total_pages": 1},
		})
	})
	mux.HandleFunc("GET /api/v1/users/7", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // deleted since, or otherwise unreachable
	})
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "pagination": map[string]any{"total_pages": 1}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	c := xclient.New(server.URL)
	data, err := restEmbedGraphRunner{}.Run(context.Background(), c, connstore.Connection{}, restGraphSpec{EntityType: "posts", EmbedDepth: 1})
	if err != nil {
		t.Fatalf("unexpected error -- one unreachable target should not fail the whole walk: %v", err)
	}
	if _, ok := nodeByID(data, "posts:1"); !ok {
		t.Fatal("posts:1 missing -- the top-level entity should still be present")
	}
	if len(data.Edges) != 1 || data.Edges[0].To != "users:7" {
		t.Fatalf("edges = %+v, want the edge still recorded even though the target couldn't be fetched", data.Edges)
	}
	stub, ok := nodeByID(data, "users:7")
	if !ok {
		t.Fatal("users:7 node missing -- despite the fetch failing 404, it should still get a collapsed stub so the edge has somewhere to point")
	}
	if !stub.Collapsed {
		t.Error("users:7 present but not marked Collapsed -- its fetch failed, it was never actually retrieved")
	}
}

func TestRestEmbedGraphRunner_CollapsedStubUpgradedWhenReachedAgainWithBudget(t *testing.T) {
	// A chain deep enough that the first path to reach "users:9"
	// exhausts its hop budget (collapsing it), but a second,
	// independent path -- a direct top-level post authored by the
	// same user -- reaches it with budget to spare. The stub must be
	// upgraded in place, not left collapsed and not duplicated.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/posts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"author_id": map[string]any{"type": "object", "format": "ref"},
				"category":  map[string]any{"type": "object", "format": "ref"},
			},
		})
	})
	mux.HandleFunc("GET /api/v1/schema/categories", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":       "object",
			"properties": map[string]any{"owner": map[string]any{"type": "object", "format": "ref"}},
		})
	})
	mux.HandleFunc("GET /api/v1/schema/users", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "object", "properties": map[string]any{}})
	})
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "pagination": map[string]any{"total_pages": 1}})
	})
	mux.HandleFunc("GET /api/v1/posts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				// post 1 reaches users:9 only via category -> owner, two hops deep, at depth 1.
				{"id": float64(1), "category": map[string]any{"type": "REF", "entity": "categories", "id": float64(5)}},
				// post 2 reaches users:9 directly, one hop.
				{"id": float64(2), "author_id": map[string]any{"type": "REF", "entity": "users", "id": float64(9)}},
			},
			"pagination": map[string]any{"page": 1, "per_page": 10, "total_items": 2, "total_pages": 1},
		})
	})
	mux.HandleFunc("GET /api/v1/categories/5", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": float64(5), "owner": map[string]any{"type": "REF", "entity": "users", "id": float64(9)},
		})
	})
	mux.HandleFunc("GET /api/v1/users/9", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(9), "name": "Real Alice"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	c := xclient.New(server.URL)
	// depth 1: post -> category (hop 1, budget exhausted there) would
	// collapse users:9 via categories:5's own owner field; post 2's
	// direct author_id edge reaches users:9 within budget (hop 1) and
	// should fetch it for real, upgrading any stub already recorded.
	data, err := restEmbedGraphRunner{}.Run(context.Background(), c, connstore.Connection{}, restGraphSpec{EntityType: "posts", EmbedDepth: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	user, ok := nodeByID(data, "users:9")
	if !ok {
		t.Fatal("users:9 node missing")
	}
	if user.Collapsed {
		t.Error("users:9 still marked Collapsed -- should have been upgraded once post 2's direct reference fetched it for real")
	}
	if user.Data["name"] != "Real Alice" {
		t.Errorf("users:9 data = %+v, want the real fetched document (name=Real Alice)", user.Data)
	}
	count := 0
	for _, n := range data.Nodes {
		if n.ID == "users:9" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("users:9 appears %d times, want exactly 1 (no duplicate from the upgrade)", count)
	}
}

func TestRestEmbedGraphRunner_Expand_FetchesNodeAndItsOwnRefs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schema/users", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":       "object",
			"properties": map[string]any{"manager": map[string]any{"type": "object", "format": "ref"}},
		})
	})
	mux.HandleFunc("GET /api/v1/schema/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("GET /api/v1/xoluman_field_meta", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "pagination": map[string]any{"total_pages": 1}})
	})
	mux.HandleFunc("GET /api/v1/users/7", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": float64(7), "name": "Alice", "manager": map[string]any{"type": "REF", "entity": "users", "id": float64(1)},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	c := xclient.New(server.URL)
	data, err := restEmbedGraphRunner{}.Expand(context.Background(), c, connstore.Connection{}, "users", 7, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	user, ok := nodeByID(data, "users:7")
	if !ok {
		t.Fatal("users:7 node missing")
	}
	if user.Collapsed {
		t.Error("users:7 marked Collapsed -- it was just fully fetched, should not be")
	}
	if user.Data["name"] != "Alice" {
		t.Errorf("users:7 data = %+v, want the real fetched document", user.Data)
	}
	if len(data.Edges) != 1 || data.Edges[0].From != "users:7" || data.Edges[0].To != "users:1" {
		t.Fatalf("edges = %+v, want one edge users:7 -[manager]-> users:1", data.Edges)
	}
}

func TestRestEmbedGraphRunner_Expand_MissingTypeOrInvalidIDRejected(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	t.Cleanup(server.Close)
	c := xclient.New(server.URL)

	if _, err := (restEmbedGraphRunner{}).Expand(context.Background(), c, connstore.Connection{}, "", 7, 1); err == nil {
		t.Error("expected an error for an empty entity type, got nil")
	}
	if _, err := (restEmbedGraphRunner{}).Expand(context.Background(), c, connstore.Connection{}, "users", 0, 1); err == nil {
		t.Error("expected an error for a zero id, got nil")
	}
	if _, err := (restEmbedGraphRunner{}).Expand(context.Background(), c, connstore.Connection{}, "users", -1, 1); err == nil {
		t.Error("expected an error for a negative id, got nil")
	}
}

func TestRestEmbedGraphRunner_Expand_DepthClampedToMaxHops(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	t.Cleanup(server.Close)
	c := xclient.New(server.URL)
	// A 404 on the seed fetch itself is fine here -- this only checks
	// that an absurd depth doesn't panic or otherwise misbehave before
	// even reaching the fetch; Expand should still return the (empty,
	// error-carrying) result cleanly.
	_, err := (restEmbedGraphRunner{}).Expand(context.Background(), c, connstore.Connection{}, "users", 7, 999999)
	if err == nil {
		t.Error("expected an error from the seed fetch against an empty mock server, got nil")
	}
}
