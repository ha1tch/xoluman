#!/usr/bin/env python3
"""repoman/config.py — repository root discovery and configuration.

A repository opts in by carrying `.repoman.json` at its root (an empty
object is a valid opt-in: every key has a default). Root discovery
walks upward from the current directory for `.repoman.json`, then for
`.git`, else uses the current directory.

Defaults encode a documented set of repository conventions (tracking
register, resolution record, known-issues/dormant-guards documents,
plain VERSION file); any of them can be overridden per repository.
"""

import json
from pathlib import Path

DEFAULTS = {
    "id_prefix": "T",
    "tracking": "docs/TRACKING.md",
    "resolved": "docs/RESOLVED.md",
    "known_issues": "docs/KNOWN_ISSUES.md",
    "changelog": "CHANGELOG.md",
    "version_file": "VERSION",
    # Extra files that must carry the version; each entry:
    #   {"file": path, "match": regex-with-one-capture-group}
    "version_targets": [],
    "release": {
        "steps": [],          # see relcore.py for the step schema
        "archive": {},        # see relcore.py step_archive
    },
}


def find_root(start: Path | None = None) -> Path:
    p = (start or Path.cwd()).resolve()
    for candidate in [p, *p.parents]:
        if (candidate / ".repoman.json").is_file():
            return candidate
    for candidate in [p, *p.parents]:
        if (candidate / ".git").exists():
            return candidate
    return p


def load(root: Path | None = None) -> tuple[Path, dict]:
    root = root or find_root()
    cfg = json.loads(json.dumps(DEFAULTS))  # deep copy
    f = root / ".repoman.json"
    if f.is_file():
        user = json.loads(f.read_text())
        for k, v in user.items():
            if isinstance(v, dict) and isinstance(cfg.get(k), dict):
                cfg[k].update(v)
            else:
                cfg[k] = v
    return root, cfg
