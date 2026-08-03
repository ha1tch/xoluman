#!/usr/bin/env python3
"""selftest.py — repoman's acceptance gate.

Builds a synthetic repository in a temp directory and exercises every
tool against it: ed (its own nine-path selftest), roles, syncver
(set/check/regex target), register (add/close/check round-trip),
guards (list/stale/record), and relcore (full run, failure halt,
resume, archive with manifest + contamination guard). Exit 0 green.
"""

import json
import os
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
PKG = HERE / "repoman"


def run(tool, *args, cwd=None):
    return subprocess.run([sys.executable, str(PKG / tool), *args],
                          capture_output=True, text=True, cwd=cwd)


def main() -> int:
    checks = 0

    def ok(cond, label, detail=""):
        nonlocal checks
        if not cond:
            print(f"FAIL: {label}\n{detail}")
            sys.exit(1)
        checks += 1
        print(f"ok  {label}")

    # 1. ed's own selftest.
    r = run("ed.py", "selftest")
    ok(r.returncode == 0 and "9 paths green" in r.stdout, "ed selftest",
       r.stdout + r.stderr)

    with tempfile.TemporaryDirectory() as d:
        root = Path(d)
        os.chdir(root)
        (root / "docs").mkdir()
        (root / ".repoman.json").write_text(json.dumps({
            "id_prefix": "Q",
            "version_targets": [
                {"file": "app.py",
                 "match": r'VERSION = "([0-9.]+)"'}],
            "release": {
                "steps": [
                    {"name": "sync", "builtin": "syncver", "always": True},
                    {"name": "build", "run": "echo building > built.txt",
                     "resumable": True},
                    {"name": "archive", "builtin": "archive", "always": True},
                ],
                "archive": {"sources": ["VERSION", "app.py", "docs"],
                            "exclude": ["*.secret"]},
            },
        }))
        (root / "VERSION").write_text("0.0.1\n")
        (root / "app.py").write_text('VERSION = "0.0.1"\n')
        (root / "CHANGELOG.md").write_text(
            "## [0.0.2] - 2026-01-02\n\n## [0.0.1] - 2026-01-01\n")
        (root / "docs" / "TRACKING.md").write_text(
            "# Register\n\nVersion: 0.0.1\n\n## Status table\n\n"
            "| ID | Summary | Theme | Priority | Status | Blocks |\n"
            "|---|---|---|---|---|---|\n"
            "| Q-01 | seed item | core | P2 | ☐ | — |\n\n"
            "## core\n\n### Q-01. seed item\n\n"
            "Theme: core · Priority: P2 · Status: ☐\n\n"
            "- **Trigger:** fixture.\n\n---\n")
        (root / "docs" / "RESOLVED.md").write_text(
            "# Resolved\n\nClosed items, newest first.\n\n"
            "## [0.0.0] Q-00 — genesis (v0.0.0, 2026-01-01)\n\ndone.\n")
        (root / "docs" / "KNOWN_ISSUES.md").write_text(
            "# Known issues\n\nVersion: 0.0.1\n\n## Dormant guards\n\n"
            "### G-01. fixture guard (`x_test.go`)\n\n"
            "- **Gate:** build tag `stress`\n"
            "- **Invocation:** `go test -tags stress ./...`\n"
            "- **Last exercised:** 2025-12-01 env:m1\n")

        # 2. syncver: set writes both the file and the regex target.
        r = run("syncver.py", "set", "0.0.2")
        ok(r.returncode == 0 and 'VERSION = "0.0.2"' in
           (root / "app.py").read_text(), "syncver set + regex target",
           r.stderr)
        r = run("syncver.py", "check")
        ok(r.returncode == 0, "syncver check", r.stdout + r.stderr)

        # 3. register with a non-default prefix: add, check, close.
        r = run("register.py", "add", "--summary", "second", "--theme",
                "core", "--priority", "P3", "--body", "- **Trigger:** t.")
        ok(r.returncode == 0 and "Q-02" in r.stdout, "register add Q-02",
           r.stdout + r.stderr)
        r = run("register.py", "check")
        ok(r.returncode == 0 and "2 open" in r.stdout, "register check",
           r.stdout)
        r = run("register.py", "close", "Q-02", "--version", "0.0.2")
        ok(r.returncode == 0, "register close", r.stdout + r.stderr)
        r = run("register.py", "check")
        ok(r.returncode == 0 and "1 open" in r.stdout,
           "close removed row AND detail", r.stdout)
        ok("Q-02" in (root / "docs" / "RESOLVED.md").read_text(),
           "closure recorded in RESOLVED")

        # 4. guards: list, stale against previous release date, record.
        r = run("guards.py", "list")
        ok("G-01" in r.stdout, "guards list", r.stdout + r.stderr)
        r = run("guards.py", "stale")
        ok(r.returncode == 1 and "G-01" in r.stdout,
           "stale detects unexercised guard", r.stdout)
        r = run("guards.py", "record", "G-01", "--date", "2026-01-03",
                "--env", "ci")
        ok(r.returncode == 0, "guards record", r.stderr)
        r = run("guards.py", "stale")
        ok(r.returncode == 0, "stale clean after record", r.stdout)

        # 5. relcore: full run, journal, archive with manifest.
        (root / "leak.secret").write_text("x")  # excluded, proves policy
        r = run("relcore.py", "0.0.2")
        ok(r.returncode == 0 and "release v0.0.2 prepared" in r.stdout,
           "relcore full run", r.stdout + r.stderr)
        zips = list(root.glob("*-v0.0.2-checkpoint.zip"))
        ok(len(zips) == 1, "archive produced")
        with zipfile.ZipFile(zips[0]) as z:
            names = z.namelist()
            ok("MANIFEST.sha256" in names, "manifest embedded")
            ok(not any("secret" in n for n in names), "exclusion honoured")

        # 6. relcore: failing step halts; --resume skips the green build.
        cfgf = root / ".repoman.json"
        cfg = json.loads(cfgf.read_text())
        cfg["release"]["steps"].insert(
            2, {"name": "breaker", "run": "exit 3", "resumable": True})
        cfgf.write_text(json.dumps(cfg))
        r = run("relcore.py", "0.0.2")
        ok(r.returncode == 1 and "FAIL breaker" in r.stdout,
           "failure halts the run", r.stdout)
        cfg["release"]["steps"] = [s for s in cfg["release"]["steps"]
                                   if s["name"] != "breaker"]
        cfgf.write_text(json.dumps(cfg))
        r = run("relcore.py", "0.0.2", "--resume")
        ok(r.returncode == 0 and "build: journaled green, skipped"
           in r.stdout, "resume skips journaled step", r.stdout)

    print(f"selftest: all {checks} checks green")
    return 0


if __name__ == "__main__":
    sys.exit(main())
