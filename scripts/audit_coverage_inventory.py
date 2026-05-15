#!/usr/bin/env python3
"""Inventory mutating Go HTTP handlers and likely audit coverage.

This is a lightweight static scanner intended to support the audit coverage
matrix. It is intentionally conservative: it flags handlers without obvious
central audit calls so humans can decide whether the route should be audited,
domain-history-only, or explicitly allowlisted.
"""

from __future__ import annotations

import argparse
import pathlib
import re
import sys
from dataclasses import dataclass

MUTATING_NAME_MARKERS = (
    "Create",
    "Update",
    "Delete",
    "Remove",
    "Add",
    "Revoke",
    "Activate",
    "Deactivate",
    "Upload",
    "Import",
    "Export",
    "Reset",
    "Change",
    "Execute",
    "Transition",
    "Assign",
    "Toggle",
    "Start",
    "Stop",
    "Approve",
    "Reject",
    "Cancel",
)

AUDIT_MARKERS = (
    "LogAudit",
    "auditor.Log",
    "auditor.LogWithDetails",
    "auditor.LogFailure",
    "auditor.LogDenied",
    "logAudit(",
    "logAuditWithDetails(",
    "logAuditFailure(",
    "logAuditDenied(",
    "LogAuditEvent",
    "h.audit(",
)

FUNC_RE = re.compile(
    r"func \(h \*[^)]*\) (?P<name>[A-Z][A-Za-z0-9_]*)\(w http\.ResponseWriter, r \*http\.Request\) \{"
)


@dataclass
class HandlerInventoryRow:
    file: str
    line: int
    handler: str
    audited: bool


def line_no(text: str, idx: int) -> int:
    return text.count("\n", 0, idx) + 1


def find_matching_brace(text: str, start: int) -> int:
    depth = 1
    i = start
    while i < len(text) and depth:
        if text[i] == "{":
            depth += 1
        elif text[i] == "}":
            depth -= 1
        i += 1
    return i


def handler_has_mutating_name(name: str) -> bool:
    return any(marker in name for marker in MUTATING_NAME_MARKERS)


def scan_handlers(root: pathlib.Path) -> list[HandlerInventoryRow]:
    rows: list[HandlerInventoryRow] = []
    handlers_dir = root / "internal" / "handlers"
    for path in sorted(handlers_dir.glob("*.go")):
        text = path.read_text(errors="ignore")
        for m in FUNC_RE.finditer(text):
            name = m.group("name")
            if not handler_has_mutating_name(name):
                continue
            body_start = m.end()
            body_end = find_matching_brace(text, body_start)
            body = text[m.start() : body_end]
            audited = any(marker in body for marker in AUDIT_MARKERS)
            rows.append(
                HandlerInventoryRow(
                    file=str(path.relative_to(root)),
                    line=line_no(text, m.start()),
                    handler=name,
                    audited=audited,
                )
            )
    return rows


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", default=".", help="repository root (default: current directory)")
    parser.add_argument("--missing-only", action="store_true", help="only print handlers without detected audit calls")
    args = parser.parse_args()

    root = pathlib.Path(args.root).resolve()
    rows = scan_handlers(root)
    print("file:line\thandler\taudited")
    for row in rows:
        if args.missing_only and row.audited:
            continue
        print(f"{row.file}:{row.line}\t{row.handler}\t{str(row.audited).lower()}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
