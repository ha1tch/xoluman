Filed: 2026-08-04

# To the xolu team — schema endpoints and tenant-scoped clients

## The bug, precisely

`Client.do()` (`pkg/client/client.go`) applies the tenant path prefix
to *every* request whenever a tenant is configured on the client, with
no per-endpoint awareness of which endpoints are actually
tenant-scoped:

```go
func (c *Client) buildURL(path string) string {
	if c.tenantID != "" {
		return fmt.Sprintf("%s/api/v1/tenant/%s%s", c.baseURL, c.tenantID, path)
	}
	return fmt.Sprintf("%s/api/v1%s", c.baseURL, path)
}
```

`GetEntitySchema`/`DefineEntitySchema` (`pkg/client/schema.go`) both go
through this — `c.do(ctx, http.MethodGet, "/schema/"+entityType, ...)`.
But `/schema/{entity}` is registered on the server only once
(`pkg/server/server.go`), outside the tenant router group — confirmed
directly, not inferred: no tenant-scoped duplicate exists for it, unlike
`/entities`, `/entity/{entity}/schema-suggestion`, and both promote
endpoints, which genuinely are registered under both the root and
tenant-scoped router groups.

The result, for any client with a tenant configured: a schema fetch for
`companies` requests `/api/v1/tenant/{tenant}/schema/companies` — a path
that doesn't exist as such. Your own router matches it against the
entity-by-id pattern instead (`/tenant/{tenantID}/{entity}/{id}`),
landing `"schema"` in `{entity}` and `"companies"` in the numeric `{id}`
slot. `strconv.Atoi("companies")` fails. `XOLU-ST004: Invalid ID`.

## How this was found

Reported by a real user of xoluman as "clicking on any entity gives a
400" — genuinely difficult to reproduce for a long stretch, since it
only manifests when a connection has a tenant configured, and every
early test case used `AuthType=none` with no tenant set. Reproduced
conclusively by running `examples/crm` (thank you for shipping that —
it was exactly the realistic, tenant-scoped, multi-entity-type dataset
needed to surface this) and confirmed with a direct curl comparison:

```bash
# The malformed URL a tenant-scoped client actually sends:
curl http://127.0.0.1:8080/api/v1/tenant/acme_crm/schema/companies
# {"error":{"code":"XOLU-ST004","message":"Invalid ID","status":400}}

# The correct, working global schema URL:
curl http://127.0.0.1:8080/api/v1/schema/companies
# {"additionalProperties":false,"properties":{...},"required":[...],"type":"object"}
```

## What we'd ask for

Either of these closes it — genuinely don't have a strong preference,
whichever fits your own architecture better:

1. `buildURL` (or a schema-specific path builder) knows `/schema/...`
   is never tenant-scoped and never applies the prefix to it, or
2. `GetEntitySchema`/`DefineEntitySchema` build their own tenant-less
   URL directly rather than going through the shared `c.do()`/
   `buildURL()` path.

Whichever you prefer — this is squarely a client-library-internal
routing decision, not something we need a say in.

## What we did in the meantime

xoluman's own workaround, not a request to change anything about
this ask: `internal/xoluext.BuildSchemaClient()` constructs a second
client per connection, identical to the normal one except with no
tenant configured, used only for the two schema calls. Every other
xolu client method used elsewhere in xoluman is unaffected — this is
narrowly scoped to the two calls that actually hit the global
endpoint. Not something we'd want to keep maintaining long-term if the
client library itself closes the gap; happy to drop it the moment a
fixed client version ships.
