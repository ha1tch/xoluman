#!/usr/bin/env python3
"""
repoman/register.py — mechanized operations on the live register
(docs/TRACKING.md) and resolution record (docs/RESOLVED.md).

DIVERGENCE FROM UPSTREAM (xoluman, 2026-08-03): cmd_close's RESOLVED.md
insertion now falls back to appending after the intro prose when no
'## ' entry exists yet, instead of refusing — a brand-new repository's
first-ever closure has nothing to insert before. Not yet reported
upstream to github.com/ha1tch/repoman.

The closure procedure (TRACKING_PRACTICES.md, "Closure procedure") is
precisely specified and repeatedly hand-executed — which is exactly
where hand-editing mistakes live. This tool makes the procedure's
violations unmakeable rather than detected after the fact.

Commands:

    list                       one line per open item
    show T-nn                  print an item's detail section
    add  --summary S --theme T --priority Pn [--status ☐]
         [--blocks TEXT] [--body TEXT | --body-file F] [--dry-run]
                               file a new item: next free id, row +
                               detail section inserted together
    close T-nn --version X.Y.Z [--date YYYY-MM-DD] [--dry-run]
                               closure procedure: detail text moved to
                               the top of RESOLVED.md stamped with
                               version+date; row AND detail removed
                               from the register in the same operation
    check                      register consistency (the release gate's
                               A1–A3 delegate here — one implementation,
                               so the gate and this editor cannot
                               disagree about what consistent means)

Always run `check` (or the release gate) after any manual edit to the
register. `--dry-run` prints what would change without writing.

Status symbols: ✓ done · ◐ partial · ☐ not started · ✗ dropped.
A ✓ item in the register is itself a defect — close it instead.
"""

import argparse
import datetime
import difflib
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import config as _config

_ROOT, _CFG = _config.load()
TRACKING = _ROOT / _CFG["tracking"]
RESOLVED = _ROOT / _CFG["resolved"]
_P = _CFG["id_prefix"]

STATUS_SYMBOLS = {"✓", "◐", "☐", "✗"}
ID_RE = re.compile(_P + r"-\d+")
ROW_RE = re.compile(r"^\| (" + _P + r"-\d+) \|")
HEAD_RE = re.compile(r"^### (" + _P + r"-\d+)\. (.*)$", re.M)
FIELD_RE = re.compile(
    r"^Theme: (\S+) · Priority: \*{0,2}(P\d)\*{0,2} · Status: (\S+)(?: · Blocks/after: (.*))?$",
    re.M)


class Item:
    def __init__(self, tid, title, theme, priority, status, blocks, body):
        self.tid, self.title = tid, title
        self.theme, self.priority, self.status = theme, priority, status
        self.blocks = blocks or ""
        self.body = body  # detail text below the field line, verbatim

    def field_line(self):
        s = f"Theme: {self.theme} · Priority: {self.priority} · Status: {self.status}"
        if self.blocks:
            s += f" · Blocks/after: {self.blocks}"
        return s


class Register:
    """Parsed view of TRACKING.md. parse() and render-back are
    edit-in-place on the original text (surgical splices), so untouched
    content — including formatting this parser does not model — is
    preserved byte-for-byte."""

    def __init__(self, text: str):
        self.text = text
        self.rows = {}     # tid -> (theme, pri, status, raw row line)
        self.items = {}    # tid -> Item
        self.spans = {}    # tid -> (start, end) offsets of the detail section
        self._parse()

    def _parse(self):
        for m in re.finditer(r"^\| (" + _P + r"-\d+) \|([^|]*)\|([^|]*)\|([^|]*)\|([^|]*)\|.*$",
                             self.text, re.M):
            self.rows[m.group(1)] = (m.group(3).strip(),
                                     m.group(4).strip().strip('*'),
                                     m.group(5).strip(), m.group(0))
        heads = list(HEAD_RE.finditer(self.text))
        for i, h in enumerate(heads):
            start = h.start()
            # section ends at the next ### / ## heading or trailing ---
            tail = self.text[h.end():]
            nm = re.search(r"^(### |## |---\s*$)", tail, re.M)
            end = h.end() + (nm.start() if nm else len(tail))
            block = self.text[start:end]
            fm = FIELD_RE.search(block)
            if not fm:
                continue
            body_start = block.index(fm.group(0)) + len(fm.group(0))
            self.items[h.group(1)] = Item(
                h.group(1), h.group(2).strip(), fm.group(1), fm.group(2),
                fm.group(3), fm.group(4), block[body_start:].rstrip() + "\n")
            self.spans[h.group(1)] = (start, end)

    def next_id(self) -> str:
        n = len(_P) + 1
        ids = [int(t[n:]) for t in set(self.rows) | set(self.items)]
        return f"{_P}-{(max(ids) + 1) if ids else 1:02d}"

    def check(self, r) -> None:
        """A1–A3 plus symbol validity. `r` needs .err(section, msg) and
        .warn(section, msg) — the release gate's Report qualifies."""
        for tid, (_, _, status, raw) in self.rows.items():
            if "✓" in raw:
                r.err("A1", f"closed item in register: {raw.strip()[:70]}")
        only_rows = set(self.rows) - set(self.items)
        only_items = set(self.items) - set(self.rows)
        if only_rows:
            r.err("A2", f"rows without detail blocks: {sorted(only_rows)}")
        if only_items:
            r.err("A2", f"detail blocks without rows: {sorted(only_items)}")
        for tid in set(self.rows) & set(self.items):
            it = self.items[tid]
            row = self.rows[tid][:3]
            det = (it.theme, it.priority, it.status)
            if row != det:
                r.err("A3", f"{tid}: table {row} vs detail {det}")
            if it.status not in STATUS_SYMBOLS:
                r.err("A3", f"{tid}: unknown status symbol {it.status!r}")


def load() -> Register:
    return Register(TRACKING.read_text())


def write_with_diff(path: Path, new_text: str, dry: bool, label: str) -> None:
    old = path.read_text() if path.exists() else ""
    if old == new_text:
        print(f"   no change: {path.name}")
        return
    if dry:
        diff = difflib.unified_diff(old.splitlines(keepends=True),
                                    new_text.splitlines(keepends=True),
                                    fromfile=f"a/{path.name}", tofile=f"b/{path.name}")
        sys.stdout.writelines(list(diff)[:80])
        print(f"   (dry-run) {label}: {path.name} not written")
        return
    path.write_text(new_text)
    print(f"   {label}: {path.name} updated")


def cmd_list(reg: Register) -> int:
    for tid in sorted(reg.items, key=lambda t: int(t[2:])):
        it = reg.items[tid]
        print(f"{tid}  {it.status}  {it.priority}  [{it.theme}]  {it.title}")
    return 0


def cmd_show(reg: Register, tid: str) -> int:
    if tid not in reg.items:
        print(f"no such item: {tid}", file=sys.stderr)
        return 1
    s, e = reg.spans[tid]
    print(reg.text[s:e].rstrip())
    return 0


def cmd_add(reg: Register, args) -> int:
    tid = args.id or reg.next_id()
    if tid in reg.rows or tid in reg.items:
        print(f"id already exists: {tid} (ids are never reused)", file=sys.stderr)
        return 1
    if args.status not in STATUS_SYMBOLS:
        print(f"invalid status {args.status!r}; use one of {STATUS_SYMBOLS}",
              file=sys.stderr)
        return 1
    body = args.body or ""
    if args.body_file:
        body = Path(args.body_file).read_text()
    if not body.strip():
        print("a register item needs a body (--body / --body-file): at "
              "minimum a Trigger line and a Scope line", file=sys.stderr)
        return 1

    text = reg.text
    # Row: insert after the last existing T-row in the status table.
    row_matches = list(re.finditer(r"^\| " + _P + r"-\d+ \|.*$", text, re.M))
    if not row_matches:
        print("cannot locate the status table", file=sys.stderr)
        return 1
    last = row_matches[-1]
    row = (f"| {tid} | {args.summary} | {args.theme} | {args.priority} "
           f"| {args.status} | {args.blocks or '—'} |")
    text = text[:last.end()] + "\n" + row + text[last.end():]

    # Detail: into the matching `## <theme>` group, else a new group at
    # the tail (before the trailing --- if present).
    section = (f"### {tid}. {args.summary}\n\n"
               f"Theme: {args.theme} · Priority: {args.priority} · "
               f"Status: {args.status}"
               + (f" · Blocks/after: {args.blocks}" if args.blocks else "")
               + "\n\n" + body.rstrip() + "\n\n")
    gm = re.search(rf"^## {re.escape(args.theme)}\s*$", text, re.M)
    if gm:
        tail = text[gm.end():]
        nm = re.search(r"^## |^---\s*$", tail, re.M)
        ins = gm.end() + (nm.start() if nm else len(tail))
        text = text[:ins] + section + text[ins:]
    else:
        tm = re.search(r"^---\s*$", text[::-1], re.M)
        if text.rstrip().endswith("---"):
            idx = text.rstrip().rfind("\n---")
            text = text[:idx] + f"\n## {args.theme}\n\n" + section + text[idx:]
        else:
            text = text.rstrip() + f"\n\n## {args.theme}\n\n" + section
    write_with_diff(TRACKING, text, args.dry_run, f"add {tid}")
    if not args.dry_run:
        print(f"filed {tid}; run `register.py check` — and remember the "
              "status table and field lines must not diverge")
    return 0


def cmd_close(reg: Register, args) -> int:
    tid = args.item
    if tid not in reg.items or tid not in reg.spans:
        print(f"no such item in the register: {tid}", file=sys.stderr)
        return 1
    if tid not in reg.rows:
        print(f"{tid} has a detail section but no table row — fix A2 first",
              file=sys.stderr)
        return 1
    it = reg.items[tid]
    date = args.date or datetime.date.today().isoformat()
    version = args.version

    # 1. Resolution entry: full detail text as at closure, stamped.
    entry = (f"## [{version}] {tid} — {it.title} (v{version}, {date})\n\n"
             f"Theme: {it.theme} · closed {version} · {date}\n"
             f"{it.body.rstrip()}\n\n"
             f"Cross-ref: CHANGELOG {version}.\n\n")
    resolved = RESOLVED.read_text()
    first = re.search(r"^## ", resolved, re.M)
    if first:
        new_resolved = resolved[:first.start()] + entry + resolved[first.start():]
    else:
        # Bootstrap case, divergence from upstream repoman: a brand-new
        # repository's RESOLVED.md has no '## ' entry yet for the first
        # closure to insert before. Append after the file's intro prose
        # instead of refusing — every fresh repo's very first close hits
        # this exactly once. Report upstream to github.com/ha1tch/repoman.
        new_resolved = resolved.rstrip("\n") + "\n\n" + entry

    # 2. Register: remove detail section and row in one operation.
    s, e = reg.spans[tid]
    text = reg.text[:s] + reg.text[e:]
    raw_row = reg.rows[tid][3]
    text = text.replace(raw_row + "\n", "", 1)
    # Drop a theme group emptied by the removal.
    text = re.sub(rf"^## {re.escape(it.theme)}\s*\n+(?=(## |---\s*$))", "",
                  text, flags=re.M)

    write_with_diff(RESOLVED, new_resolved, args.dry_run, f"close {tid} (record)")
    write_with_diff(TRACKING, text, args.dry_run, f"close {tid} (register)")
    if not args.dry_run:
        print(f"closed {tid} at v{version}. Remaining by hand: the CHANGELOG "
              f"entry for {version} should cross-reference this closure "
              "(the changelog says what shipped; RESOLVED.md says what was "
              "wrong — they reference, never duplicate).")
    return 0


def cmd_check(reg: Register) -> int:
    class R:
        def __init__(self):
            self.errors = []

        def err(self, s, m):
            self.errors.append(f"[{s}] {m}")

        def warn(self, s, m):
            print(f"WARN [{s}] {m}")

    r = R()
    reg.check(r)
    for e in r.errors:
        print(f"ERROR {e}")
    if r.errors:
        print(f"REGISTER CHECK FAIL: {len(r.errors)} error(s)")
        return 1
    print(f"REGISTER CHECK OK: {len(reg.items)} open item(s)")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(
        description="Live-register operations (docs/TRACKING.md)",
        epilog="See module docstring for the closure procedure this enforces.")
    sub = ap.add_subparsers(dest="cmd", required=True)
    sub.add_parser("list")
    p = sub.add_parser("show")
    p.add_argument("item")
    p = sub.add_parser("add")
    p.add_argument("--id", help="explicit id (default: next free)")
    p.add_argument("--summary", required=True)
    p.add_argument("--theme", required=True)
    p.add_argument("--priority", required=True, help="P1 (highest) .. P4")
    p.add_argument("--status", default="☐")
    p.add_argument("--blocks", default="")
    p.add_argument("--body")
    p.add_argument("--body-file")
    p.add_argument("--dry-run", action="store_true")
    p = sub.add_parser("close")
    p.add_argument("item")
    p.add_argument("--version", required=True)
    p.add_argument("--date")
    p.add_argument("--dry-run", action="store_true")
    sub.add_parser("check")
    args = ap.parse_args()

    reg = load()
    if args.cmd == "list":
        return cmd_list(reg)
    if args.cmd == "show":
        return cmd_show(reg, args.item)
    if args.cmd == "add":
        return cmd_add(reg, args)
    if args.cmd == "close":
        return cmd_close(reg, args)
    if args.cmd == "check":
        return cmd_check(reg)
    return 1


if __name__ == "__main__":
    sys.exit(main())
