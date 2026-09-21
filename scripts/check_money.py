#!/usr/bin/env python3
# Copyright 2026 shing1211
# SPDX-License-Identifier: Apache-2.0
"""check_money.py — enforce ADR 0008: money/quantity fields are never floats.

Money, prices, quantities, and rates must be represented as string or
json.Number, never as binary floats. This scans exported struct fields under
pkg/ (excluding tests) and fails if any field whose name looks monetary or
quantitative is declared as float32/float64 — optionally via a pointer, slice,
or array.

Exits 0 (with a notice) when pkg/ does not exist yet, so `make check` stays
green while the repo is still a skeleton.
"""

import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PKG_DIR = os.path.join(ROOT, "pkg")

# Exported field declaration(s): one or more names, then the type (+ optional tag).
FIELD = re.compile(
    r"^\s+([A-Z][A-Za-z0-9_]*(?:\s*,\s*[A-Z][A-Za-z0-9_]*)*)\s+(.*)$"
)
FLOAT = re.compile(r"\b(float32|float64)\b")
TAG = re.compile(r"`[^`]*`")
WORDS = re.compile(r"[A-Z]+(?=[A-Z][a-z])|[A-Z]?[a-z]+|[A-Z]+|\d+")

# CamelCase tokens that indicate a monetary or quantitative value. Tokenised
# (not substring) matching avoids false positives such as "Corporate" -> rate.
MONEY_TOKENS = {
    "price", "prices", "amount", "amt", "money", "balance", "value",
    "valuation", "income", "ratio", "rate", "qty", "quantity", "quantities",
    "volume", "turnover", "fee", "fees", "commission", "cost", "profit",
    "loss", "pnl", "margin", "interest", "dividend", "cash", "fund", "funds",
    "asset", "assets", "equity", "nav", "notional", "premium", "strike",
    "tax", "charge", "charges", "debt", "credit", "last", "close", "open",
    "high", "low", "bid", "ask", "change", "delta", "spread", "settlement",
    "avg", "average",
}


def iter_go_files(root):
    for dirpath, _dirnames, filenames in os.walk(root):
        for name in filenames:
            if name.endswith(".go") and not name.endswith("_test.go"):
                yield os.path.join(dirpath, name)


def is_money_name(name):
    return any(w.lower() in MONEY_TOKENS for w in WORDS.findall(name))


def main():
    if not os.path.isdir(PKG_DIR):
        print("money-check OK: pkg/ does not exist yet")
        return 0

    violations = []
    for path in sorted(iter_go_files(PKG_DIR)):
        with open(path, encoding="utf-8") as fh:
            for lineno, line in enumerate(fh, 1):
                code = line.split("//", 1)[0]
                match = FIELD.match(code)
                if not match:
                    continue
                remainder = TAG.sub("", match.group(2)).strip()
                if "=" in remainder or not FLOAT.search(remainder):
                    continue
                for name in (n.strip() for n in match.group(1).split(",")):
                    if is_money_name(name):
                        rel = os.path.relpath(path, ROOT).replace(os.sep, "/")
                        violations.append(
                            "%s:%d: %s %s" % (rel, lineno, name, remainder)
                        )

    if violations:
        print(
            "money-check: float money/quantity fields found in pkg/ (ADR 0008):",
            file=sys.stderr,
        )
        for v in violations:
            print("  " + v, file=sys.stderr)
        print(
            "  use string or json.Number for money and quantities",
            file=sys.stderr,
        )
        return 1

    print("money-check OK: no float money/quantity fields in pkg/")
    return 0


if __name__ == "__main__":
    sys.exit(main())
