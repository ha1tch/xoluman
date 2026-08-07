# examples/crm — a medium-sized CRM dataset

Two scripts: one launches a correctly-configured xolu instance, the other seeds it with a realistic, cross-referenced CRM data model. Six entity types (users, companies, contacts, deals, activities, tasks, all linked via REF fields), plus real use of four more xolu primitives against the same domain — not disconnected toy examples: two `/fsm` machine defs (`deal_lifecycle`, `task_lifecycle`, mirroring the `stage`/`status` enums the entities already have), a `/dxp` transaction (`close_deal_won`, spanning an entity update, an entity create, and a `/bal` transfer atomically), a `/bal` revenue-recognition ledger pair, and a few `/blob` contract attachments. Also seeds a handful of genuinely useful saved queries (OQL, Sulpher, REST) against the data it just created, ready to explore through xoluman's own query and graph viewers. Useful as a demo, a smoke test after a build, or a starting point for testing against real-shaped multi-primitive data instead of a single flat table.

This example lives in xoluman's own repo, not xolu's — it moved here (rather than being maintained in both places, which risked two slightly-diverging copies of the same thing) since the saved-queries step is genuinely xoluman-specific bookkeeping (`xoluman_saved_query`, one query's own name references xoluman's graph viewer directly), and once that piece exists at all, keeping the whole example together in one place made more sense than splitting the xolu-native parts from the xoluman-specific ones across two repos.

## Prerequisites

- Go toolchain (the launcher builds `xolu` and `iolu` from source — a local checkout of [xolu](https://github.com/ha1tch/xolu)'s own source, pointed at via `--xolu-source`; xoluman doesn't vendor or embed xolu's source itself)
- `python3` with `requests` installed (`pip install requests`)

## Quick start

```bash
cd examples/crm
./launch_xolu_for_crm.sh --xolu-source /path/to/xolu --daemon --with-seed
```

One command: builds, provisions the tenant, starts the server, waits
for it to be ready, then runs the full seed (schemas, entities, the
two FSM machine defs, the close_deal_won DXP transaction, the bal
revenue accounts, contract blobs, and the saved queries) — printing
the connection details for xoluman (or curl) when it's done.
`--xolu-source` (or `XOLU_CRM_XOLU_SOURCE`) points at a local xolu
checkout to build from — required unless you pass `--skip-build` with
`XOLU_CRM_BIN_PATH` pointing at an already-built `xolu` binary instead.
`--seed-args "--scale 2"` forwards extra arguments straight through to
the seed script (`xolu_crm_seed.py --help` for the full list).

Prefer the two steps separately — to seed an already-running instance,
retry a seed without restarting the server, or point the seed script
at a xolu instance this launcher didn't start:

```bash
./launch_xolu_for_crm.sh --xolu-source /path/to/xolu --daemon
# ... wait for "ready", then in another shell or after backgrounding:
python3 xolu_crm_seed.py --tenant acme_crm --skip-tenant-create
```

The launcher prints the exact follow-up command (including `--api-key` if you started it with `--with-auth`) once the server is ready — copy that instead of retyping the above by hand if you changed any settings.

## What `launch_xolu_for_crm.sh` actually does

Builds `cmd/xolu` and `cmd/iolu` from the source tree at `--xolu-source`, provisions a tenant via `iolu tenant create --mode shared` **before** starting the server (required under `TenantMode=strict` — the server's own tenant registry loads once at boot and never re-reads it), then starts `xolu` configured with:

- `GraphMode=flat` — REF fields (used throughout this schema: company owners, contact-to-company links, deal-to-contact links) need graph support on
- `APIV2Enabled=true` — otherwise every single entity write logs a spurious warning about a missing `event_defs` table (a real, currently-unfixed xolu gap, filed as T-148)
- `TenantMode=strict`, `AuthType=none` by default (`--with-auth KEY` switches to `AuthType=apikey`)

Every setting is a plain shell variable with an `XOLU_CRM_*` environment override — see the top of the script. Data lands in `examples/crm/xolu-crm-data/` by default (`XOLU_CRM_BASE_DIR` to change it), gitignored, safe to delete between runs. `--with-seed` (optionally paired with `--seed-args "..."` to forward arguments to `xolu_crm_seed.py`) runs the seed script automatically once the server reports ready, instead of leaving that as a manual follow-up step.

## What `xolu_crm_seed.py` actually does

Registers six schemas (global, not tenant-scoped — confirmed directly against the server's own routing), then creates entities in dependency order so every REF field points at something that genuinely exists by the time it's written: users → companies → contacts → deals → activities/tasks. Deals reference a contact from the *same* company as the deal, not just any random contact.

`--scale` multiplies the default volumes (5 users, 15 companies, ~35 contacts, 25 deals, 50 activities, 20 tasks) up or down. `--seed` for a reproducible run.

## Why two separate scripts, not one

The launcher is infrastructure (build, configure, boot) — bash is the natural fit. The seed script is data assembly (schema definitions, realistic name pools, cross-referenced entity generation) — Python is the natural fit there. Keeping them as two separate scripts still means you can point the seed script at any already-running xolu instance, launcher-provisioned or not, as long as the tenant exists and the server config matches what the seed script expects (see `xolu_crm_seed.py --help` for the full prerequisites if you're running it standalone) — `--with-seed` is a convenience the launcher offers on top of that, not a merge of the two into one script.

## Known gap

The seed script's own `iolu tenant create` fallback (for running it against a server the launcher didn't start) only works correctly if the tenant is provisioned *before* that server's own most recent boot — the same `TenantMode=strict` registry-loads-once-at-startup constraint the launcher itself works around by sequencing correctly. If you hit `"Unknown tenant"` after the seed script's own iolu step reported success, restart the server and re-run with `--skip-tenant-create`, or just use the launcher, which gets the ordering right by construction.
