Version: 0.7.6
Last reviewed: 2026-08-03

# xoluman — Live Register

Open, actionable items only. Closed items move to `RESOLVED.md` in full,
per the closure procedure. See `docs/KNOWN_ISSUES.md` for intentional
limits and recorded decisions rather than open work.

## Status table

| ID | Summary | Theme | Priority | Status | Blocks/after |
|----|---------|-------|----------|--------|---------------|
| T-04 | Implement `ConnectionStore` keyring backend | connstore | P1 | ◐ | After: T-06 (closed, v0.1.0) |

## Detail

### T-04. `ConnectionStore` keyring backend

Theme: connstore · Priority: **P1** · Status: ◐ · Blocks/after: After T-06 (closed, v0.1.0)

Implemented against `github.com/zalando/go-keyring` v0.2.8:
`KeyringBackend` (`internal/connstore/keyring_backend.go`) stores
connection metadata in `connections-meta.json` and tokens in the OS
keyring under service `xoluman`, keyed by connection name. Metadata
never contains the token — verified by
`TestKeyringBackend_MetadataFileNeverContainsToken`, which asserts the
raw file bytes don't contain a known-secret value.

**Shipped:** the full behavioural test suite runs against
`keyring.MockInit()`'s in-memory provider (no OS keyring service exists
in the sandbox this was built in) — upsert semantics, `ErrNotFound`,
`ErrTokenUnavailable` when metadata exists but the keyring entry doesn't
(simulating external removal via the OS's own keyring UI), and both
rollback directions in `Save` (keyring write fails → no metadata written;
metadata write fails after a successful keyring write → the keyring
entry is rolled back). Confirmed against a genuinely absent keyring
service too: building with `-tags keyring_live` in this sandbox produces
a clear, informative failure (`exec: "dbus-launch": executable file not
found in $PATH`), not a panic or hang — exactly the failure mode the
dormant guard below exists to catch on a machine that's supposed to have
one.

**Outstanding — cannot be verified in this sandbox:** an actual
round-trip against a real OS keyring service. `-tags keyring_live`
swaps out `TestMain`'s mock initialization (`testmain_mock_test.go` /
`testmain_live_test.go`, split by build tag) so the *same* test suite in
`keyring_backend_test.go` becomes a genuine round-trip with no
duplicated test logic — it just hasn't been run against a real provider
yet. See the dormant-guard entry in `KNOWN_ISSUES.md` for the exact
invocation. T-04 closes once that's run and reported.

## query-editor

