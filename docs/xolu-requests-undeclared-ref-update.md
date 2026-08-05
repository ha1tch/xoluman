Filed: 2026-08-04

# To the xolu team — updating an entity with an existing, undeclared-target REF field always fails

## The bug, precisely

A `"format": "ref"` property that does **not** declare an explicit
`"target"` — `{"type": "object", "format": "ref"}`, no `target` key at
all — is exactly what `examples/crm`'s own seed script uses for every
ref field in all five non-`users` entity types (`owner`, `company`,
`contact`, `deal`, `primary_contact` — confirmed directly against
`xolu_crm_seed.py`'s schema definitions). Creating an entity with such
a field works fine. **Updating one that already has a value in that
field — PUT or PATCH, whether the update touches the field or not —
always fails with `XOLU-VL001`, `"id: unexpected field"`.**

## Reproduction, isolated variable by variable

Same instance, same tenant, same row throughout:

```bash
# Create works fine:
curl -X POST .../tenant/acme_crm/companies \
  -d '{"name":"New Co","industry":"technology","size":"small","owner":{"type":"REF","entity":"users","id":5}}'
# -> 201, {"id":16,"message":"Resource of entity companies created successfully"}

# PUT the same row, correct write-shape ref value, all other fields present:
curl -X PUT .../tenant/acme_crm/companies/1 \
  -d '{"name":"Summit Labs","industry":"technology","size":"small","owner":{"type":"REF","entity":"users","id":5},"phone":"...","website":"..."}'
# -> 400, {"details":["id: unexpected field"],"error":{"code":"XOLU-VL001",...}}

# PATCH the same row, not touching owner at all:
curl -X PATCH .../tenant/acme_crm/companies/1 -d '{"name":"Summit Labs PATCHED"}'
# -> 400, {"details":["id: unexpected field"],"error":{"code":"XOLU-VL001",...}}
```

The last one is the clearest signal: the PATCH body contains no `owner`
key, no nested `id`, nothing that looks like a ref value at all — the
update still fails identically. Whatever re-validates on update is
re-checking the **existing stored value** of `owner`, not anything the
client sent.

Confirmed the schema is the only variable that matters by registering
a second schema on the same instance with `target` explicitly
declared and repeating the same create-then-write sequence — that one
works at every step, PUT included:

```bash
curl -X POST .../schema/test_with_target \
  -d '{"type":"object","properties":{"name":{"type":"string"},"owner":{"type":"object","format":"ref","target":"users"}},"required":["name","owner"]}'
curl -X POST .../tenant/acme_crm/test_with_target \
  -d '{"name":"Test","owner":{"type":"REF","entity":"users","id":5}}'
# -> 201, works
```

## Why this matters beyond the demo

`examples/crm`'s own schemas don't declare `target` on any ref field —
this isn't a contrived edge case, it's what your own shipped example
does, and plausibly what a fair amount of real-world schema authoring
looks like if `target` isn't documented as effectively required. The
practical effect: any entity type shaped this way can be created, read,
and listed correctly, but **never updated again** once a value exists
in the ref field — not through xoluman, not through any client sending
the documented-correct write shape, since raw curl reproduces this with
nothing between the request and your server.

## What we'd ask for

Whatever's re-validating an existing ref value on update — most likely
something that fetches the current document (in its read/embedded
shape) and re-runs it through the same "format":"ref" validator used
for writes, which would explain why an *embedded* value (carrying `id`
plus whatever other fields the target document has) fails a check meant
for the *write* shape — needs to either skip re-validating fields the
update didn't touch, or correctly recognize its own embedded read-shape
as already valid rather than checking it against the write-shape
validator.

We don't have a client-side workaround for this one — happy to test
against a fix whenever one's available.
