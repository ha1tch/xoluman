// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package xoluext bridges xoluman's own types onto xolu's Go client
// (github.com/ha1tch/xolu/pkg/client). BuildClient is the one thing in
// here today; anything that needs xolu's client extended beyond what it
// already offers (T-01/T-02/T-03 — blob, export, raw request methods)
// belongs upstream in xolu itself, not bolted on here.
package xoluext

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/connstore"
)

// BuildClient constructs a *client.Client configured from a stored
// Connection: base URL, auth mode/token, and default tenant if set.
//
// A tenant-less variant of this (BuildSchemaClient) used to be
// needed as a workaround for a real xolu bug — GetEntitySchema/
// DefineEntitySchema being wrongly tenant-prefixed, filed as
// docs/xolu-requests-tenant-schema.md (T-23). Fixed directly in xolu
// v0.27.0 (buildURLRoot, confirmed against pkg/client/client.go's own
// doc comment on the fix) — the workaround was removed once that was
// verified, rather than kept around out of caution. See
// docs/RESOLVED.md's T-23 entry for the full history.
func BuildClient(conn connstore.Connection) *client.Client {
	return client.New(conn.BaseURL, append(authOptions(conn), tenantOption(conn)...)...)
}

func authOptions(conn connstore.Connection) []client.ClientOption {
	var opts []client.ClientOption
	switch conn.AuthMode {
	case connstore.AuthAPIKey:
		opts = append(opts, client.WithAPIKey(conn.Token))
	case connstore.AuthBearer:
		opts = append(opts, client.WithBearerToken(conn.Token))
	case connstore.AuthJWT:
		opts = append(opts, client.WithJWT(conn.Token))
	case connstore.AuthNone:
		// No auth option needed — client.New defaults to no Authorization header.
	}
	return opts
}

func tenantOption(conn connstore.Connection) []client.ClientOption {
	if conn.Tenant == "" {
		return nil
	}
	return []client.ClientOption{client.WithTenant(conn.Tenant)}
}

// ListWithEmbed lists entities with their REF fields hydrated inline,
// via the documented embed_depth REST parameter (docs/API_REFERENCE.md:
// GET /{entity} — "embed_depth: 0 for lists; embedding depth; disabled
// by default for performance") — confirmed directly against a real
// server, not assumed from the docs alone: GET /deals?embed_depth=1
// returns each deal's own company/owner/primary_contact fields as full
// nested documents, not bare {type, entity, id} pointers.
//
// pkg/client's own List has no way to request this at all — ListParams
// only ever produces per_page/page, nothing else reaches the query
// string (filed as XM-10 in the xoluman-xolu report log). This exists
// to cover that gap without every caller in xoluman needing to know
// it's a workaround: the return type is the identical *client.ListResult
// List itself returns, and every caller here is written exactly as if
// List already supported EmbedDepth. If/when it does, only this one
// function's body needs to change — swap the body for a real
// c.List(ctx, entity, &client.ListParams{EmbedDepth: embedDepth}) call —
// not any of its callers.
//
// Uses client.Raw, not a bypass of the xolu client — Raw is itself
// already xolu's own, sanctioned escape hatch for exactly this
// situation (its own doc comment: "requested for xoluman's own ad-hoc
// REST query console... for everything else an operator might want to
// try against a connected instance without reimplementing auth").
// Raw deliberately does no implicit tenant-prefixing (also its own
// doc comment, load-bearing, not an oversight) — conn is threaded
// through here specifically to build that prefix correctly, since a
// *client.Client has no way to hand its own configured tenant back
// out once built.
func ListWithEmbed(ctx context.Context, c *client.Client, conn connstore.Connection, entity string, embedDepth int, params *client.ListParams) (*client.ListResult, error) {
	q := url.Values{}
	if params != nil {
		if params.Limit > 0 {
			q.Set("per_page", strconv.Itoa(params.Limit))
		}
		if params.Offset > 0 && params.Limit > 0 {
			q.Set("page", strconv.Itoa(params.Offset/params.Limit+1))
		} else if params.Offset > 0 {
			q.Set("page", "1")
		}
		if params.Sort != "" {
			q.Set("sort", params.Sort)
		}
	}
	if embedDepth > 0 {
		q.Set("embed_depth", strconv.Itoa(embedDepth))
	}

	path := "/api/v1/"
	if conn.Tenant != "" {
		path = "/api/v1/tenant/" + url.PathEscape(conn.Tenant) + "/"
	}
	path += entity
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	resp, err := c.Raw(ctx, http.MethodGet, path, "", nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, &client.Error{HTTPStatus: resp.StatusCode, Message: string(resp.Body)}
	}

	var envelope struct {
		Data       []map[string]any `json:"data"`
		Pagination struct {
			Page       int `json:"page"`
			PerPage    int `json:"per_page"`
			TotalItems int `json:"total_items"`
			TotalPages int `json:"total_pages"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(resp.Body, &envelope); err != nil {
		return nil, fmt.Errorf("xoluext: decoding list response: %w", err)
	}

	entities := make([]client.Entity, 0, len(envelope.Data))
	for _, doc := range envelope.Data {
		id, _ := doc["id"].(float64)
		entities = append(entities, client.Entity{ID: int64(id), Data: doc})
	}
	return &client.ListResult{
		Entities:   entities,
		Page:       envelope.Pagination.Page,
		PerPage:    envelope.Pagination.PerPage,
		TotalItems: envelope.Pagination.TotalItems,
		TotalPages: envelope.Pagination.TotalPages,
	}, nil
}

// maxListPageSize is xolu's own, explicitly documented per_page
// ceiling (docs/API_REFERENCE.md: "Results per page (max 100)") — not
// a hidden quirk, a published API contract. The bug was on this
// side: earlier code in this codebase requested Limit: 1000 "to get
// everything in one call," which the server correctly rejects as
// out-of-range and replaces with its own configured default (10) —
// exactly the documented behavior, just never checked against the
// docs before writing the calling code. That single call then got
// treated as if it had returned everything, silently dropping every
// row past the tenth. The real, confirmed cause of both "saved
// queries don't work" (ListSavedQueries) and "FSMs don't save"
// (loadLayout/SaveLayout's own delete-old-row step) — an xoluman-side
// misuse of the API's own documented pagination contract, not an
// xolu defect.
const maxListPageSize = 100

// ListAll fetches every entity of the given type, following xolu's
// own pagination envelope (page/total_pages, exactly as
// client.ListResult's own doc comment describes: "holds a page of
// entities and the pagination metadata") rather than assuming a
// single call with a large Limit returns everything — it doesn't,
// see maxListPageSize's own comment. Any code that needs "all rows of
// a bookkeeping-scale entity type" (saved queries, FSM layouts, and
// anything similar added later) should call this instead of
// c.List(ctx, entity, &client.ListParams{Limit: <big number>}), which
// silently truncates at the documented 100-row ceiling (or the
// server's configured default below that) with no error and no
// indication anything was left out.
func ListAll(ctx context.Context, c *client.Client, entity string) ([]client.Entity, error) {
	var all []client.Entity
	page := 1
	for {
		result, err := c.List(ctx, entity, &client.ListParams{
			Limit:  maxListPageSize,
			Offset: (page - 1) * maxListPageSize,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, result.Entities...)
		if page >= result.TotalPages || len(result.Entities) == 0 {
			return all, nil
		}
		page++
	}
}

// GetWithEmbed fetches a single entity by ID with an explicit
// embed_depth, via the same documented REST parameter and the same
// client.Raw-based approach as ListWithEmbed (see its own doc comment
// for the full reasoning — this exists for the identical gap, XM-10,
// just on the single-entity endpoint rather than the list one).
//
// The one genuinely new reason this exists, beyond mirroring List's
// own gap: requesting embed_depth=0 explicitly is not the same as
// omitting the parameter. GET /{entity}/{id}'s own documented default
// is "enabled" (docs/API_REFERENCE.md) — so c.Get, which sends no
// query parameters at all, always gets whatever the server's default
// embedding is, with no way to ask for bare pointers instead. That
// matters here specifically: a REF field's hydrated value never
// self-identifies its own entity type (confirmed directly — an
// embedded document has no "type"/"entity" marker of its own), while
// a bare pointer always does. The graph walker (graphrest.go) needs
// bare pointers at every hop specifically so it always knows what it's
// looking at, then chooses for itself whether and how deep to keep
// hydrating from there — server-side embedding depth and the walker's
// own hop budget are two different, easily-conflated concepts, and
// conflating them (as an earlier version of graphrest.go did) silently
// produced edges to a target of unknown type whenever the field's
// schema didn't declare one, which turned out to be the common case,
// not the exception.
func GetWithEmbed(ctx context.Context, c *client.Client, conn connstore.Connection, entity string, id int64, embedDepth int) (*client.Entity, error) {
	q := url.Values{}
	q.Set("embed_depth", strconv.Itoa(embedDepth))
	if embedDepth == 0 {
		q.Set("embed", "false")
	}

	path := "/api/v1/"
	if conn.Tenant != "" {
		path = "/api/v1/tenant/" + url.PathEscape(conn.Tenant) + "/"
	}
	path += entity + "/" + strconv.FormatInt(id, 10) + "?" + q.Encode()

	resp, err := c.Raw(ctx, http.MethodGet, path, "", nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, &client.Error{HTTPStatus: resp.StatusCode, Message: string(resp.Body)}
	}

	var doc map[string]any
	if err := json.Unmarshal(resp.Body, &doc); err != nil {
		return nil, fmt.Errorf("xoluext: decoding get response: %w", err)
	}
	return &client.Entity{ID: id, Data: doc}, nil
}
