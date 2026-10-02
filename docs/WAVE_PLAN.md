# xoluman — Wave Plan

Updated: 2026-08-20

Staged-work plan for xoluman's own wave programme. Each wave's own
short pointer paragraph below explains why it exists and what it
depends on — the tracking table in `docs/WAVE_TRACKING.md` records
what actually shipped.

**Wave 1 — Seed Management Completeness (≈ 3.0d, added 2026-08-20).** The seed system (format, executor, safety check, remote source) shipped complete and independently verified across v0.7.25-0.7.31, but four real gaps were flagged along the way rather than built at the time: no settings UI, no realistic worked example beyond hello-fsm, no discoverability from the main connections UI, and the graph-rendering half of the Sulpher/graph reorganization never exercised end to end against live CAL/DXP data. Grouped as one wave because each item is small and independent, not because they block each other.

