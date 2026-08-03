# Request: `xolu/pkg/client` — FSM definition write methods

Author: Horacio, via Claude (xoluman development session)
Date: 2026-08-03
Status: draft — for the xolu team's review

---

## The gap

`pkg/server/v2_fsm_def_handlers.go` implements a complete CRUD surface
for FSM definitions:

```
POST   /api/v2/fsm/def            create a definition
GET    /api/v2/fsm/def            list definitions
GET    /api/v2/fsm/def/{id}       retrieve a definition
PUT    /api/v2/fsm/def/{id}       replace a definition (future machines only)
DELETE /api/v2/fsm/def/{id}       delete a definition (always permitted)
POST   /api/v2/fsm/def/validate   validate without storing
```

`pkg/client` wraps only the two reads — `ListMachineDefs` and
`GetMachineDef`. There's no way to create, replace, delete, or validate
a definition through the client today.

**Why now:** xoluman is building an FSM definition editor — reusing the
same Lit+canvas approach already proven in Seam AMS's own FSM editor,
extended for xolu's `guard`/`output`/`set` fields — and needs to
persist edits back to xolu. Read-only browsing works today
(`ListMachineDefs`/`GetMachineDef`); actually saving anything doesn't.

---

## Request: four methods

Shapes below are taken directly from reading
`pkg/server/v2_fsm_def_handlers.go`, not inferred from `API_V2.md` — the
lesson from the `EXPORT_API.md` mismatch found earlier this session was
to verify against the handler, not just the doc.

### `CreateMachineDef`

```go
func (c *Client) CreateMachineDef(ctx context.Context, spec MachineSpec) (*MachineDefCreateResult, error)
```

`POST /api/v2/fsm/def`. Request body is the raw `MachineSpec` (the type
already exists in `pkg/client`, used by `MachineDef.Spec`) — not
wrapped in an envelope. Response, `201 Created`:

```json
{"id": 1, "name": "...", "created_at": "...", "analysis": { ... }}
```

```go
type MachineDefCreateResult struct {
    ID        int64           `json:"id"`
    Name      string          `json:"name"`
    CreatedAt string          `json:"created_at"`
    Analysis  json.RawMessage `json:"analysis"`
}
```

`Analysis` kept opaque (`json.RawMessage`), matching how
`MachineDef.Analysis` already treats it — server-internal shape, not
modelled client-side.

### `ReplaceMachineDef`

```go
func (c *Client) ReplaceMachineDef(ctx context.Context, id int64, spec MachineSpec) (*MachineDefReplaceResult, error)
```

`PUT /api/v2/fsm/def/{id}`. Same raw-`MachineSpec` body. Response,
`200 OK`:

```json
{"id": 1, "name": "...", "analysis": { ... }}
```

```go
type MachineDefReplaceResult struct {
    ID       int64           `json:"id"`
    Name     string          `json:"name"`
    Analysis json.RawMessage `json:"analysis"`
}
```

`404` (`XOLU-FSM...` — not found) if `id` doesn't exist — should map to
`*client.Error` the same way every other method's 404 does.

Worth restating since it's easy to miss: the route comment says
"replace a definition (**future machines only**)" — i.e. this doesn't
retroactively affect already-created machine instances, only what a
*new* machine gets when created against this definition afterward.
Not asking for anything about this, just flagging it so the client
method's doc comment can say so explicitly rather than a caller
assuming a replace is retroactive.

### `DeleteMachineDef`

```go
func (c *Client) DeleteMachineDef(ctx context.Context, id int64) error
```

`DELETE /api/v2/fsm/def/{id}`. `204 No Content` on success, `404` if
`id` doesn't exist. Route comment: "delete a definition (**always
permitted**)" — no check against existing machines referencing it,
unlike what the "future machines only" language on replace might
suggest by contrast. Also just flagging, not asking for a behaviour
change.

### `ValidateMachineDef`

```go
func (c *Client) ValidateMachineDef(ctx context.Context, spec MachineSpec) (*MachineDefValidation, error)
```

`POST /api/v2/fsm/def/validate`. Same raw-`MachineSpec` body. Unlike
the other three, this always responds `200 OK` regardless of validity
— validity is in the body, not the status code:

```json
{"valid": false, "errors": [{"code": "XOLU-FSM006", "message": "..."}]}
{"valid": true, "analysis": { ... }}
```

```go
type MachineDefValidation struct {
    Valid    bool                        `json:"valid"`
    Analysis json.RawMessage             `json:"analysis,omitempty"`
    Errors   []MachineDefValidationError `json:"errors,omitempty"`
}

type MachineDefValidationError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}
```

Since this is always `200`, the method's own contract needs to be
explicit that a returned `error` means a transport/decode failure, not
an invalid spec — an invalid spec is a normal, successful response with
`Valid: false`. Worth calling out in the doc comment so a caller doesn't
reach for `errors.Is`/`*client.Error` to detect invalidity.

---

## What this unblocks, concretely

Without these, xoluman's FSM editor is read-only — browsing existing
definitions works, but there's no way to save a new one, save an edit,
remove one, or check a draft is valid before committing to it. All four
matter for that to be a real editor rather than a viewer.

Exact method names/signatures above are a starting proposal to make
this maximally actionable, not a demand — whatever shape fits the
client's existing conventions best is the right call, that's your team's
design decision.

Nothing here is implemented against your working copy. xoluman's own
tracking (`docs/TRACKING.md`, item T-13) picks up from your response.
