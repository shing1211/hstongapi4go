#!/usr/bin/env python3
# Copyright 2026 shing1211
# SPDX-License-Identifier: Apache-2.0
"""check_money.py — enforce ADR 0008: money/quantity fields are never floats.

Money, prices, quantities, and rates must be represented as:
  - string or json.Number  in pkg/  (except pkg/domain/)
  - decimal.Decimal        in pkg/domain/  (per ADR 0008)

This scans exported struct fields under pkg/ (excluding tests) and fails if any
field that looks monetary or quantitative is declared as float32/float64 —
optionally via a pointer, slice, or array.  "Looks monetary" is decided on the
Go field name *and* on the JSON wire key of its struct tag, because the wire key
is what a consumer actually reads and an innocuous Go name can hide a monetary
value behind it:

    TickSize float64 `json:"spreadLevel"`   // Go name: {Tick, Size} -> no match,
                                            // wire key: {spread, Level} -> match

Either name matching is enough to fail.  A field with no json tag, or with a tag
that carries no json key, is still judged on its Go name alone.  pkg/domain/ is
exempt from the string/json.Number rule because its types use decimal.Decimal
(ADR 0008), but float64 is still rejected everywhere.

A field is skipped only when a single entry in WAIVERS matches the complete
declaration site (path, Go name, wire key, declared type).  ADR 0008, amended
2026-09-26, grants exactly one such exception; see the WAIVERS block for its
justification and for the condition that deletes it.  Waived fields are printed
on every run, so the exception is visible in the build log rather than silent.

Exits 0 (with a notice) when pkg/ does not exist yet, so `make check` stays
green while the repo is still a skeleton.
"""

import os
import re
import sys
from collections import namedtuple

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PKG_DIR = os.path.join(ROOT, "pkg")

# Exported field declaration(s): one or more names, then the type (+ optional tag).
FIELD = re.compile(
    r"^\s+([A-Z][A-Za-z0-9_]*(?:\s*,\s*[A-Z][A-Za-z0-9_]*)*)\s+(.*)$"
)
FLOAT = re.compile(r"\b(float32|float64)\b")
TAG = re.compile(r"`[^`]*`")
WORDS = re.compile(r"[A-Z]+(?=[A-Z][a-z])|[A-Z]?[a-z]+|[A-Z]+|\d+")

# The json key of a struct tag: the `json:"spreadLevel"` of
# `TickSize float64 `json:"spreadLevel"``, capturing the value so tag options
# (`json:"spreadLevel,omitempty"`) can be split off.  Matched against the
# backticked span only, so a `-` in the type or a "//" comment cannot fake one.
JSON_TAG = re.compile(r'\bjson\s*:\s*"([^"]*)"')

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

# --------------------------------------------------------------------------
# Declared exceptions. ADR 0008 was amended on 2026-09-26 to carry exactly one.
# --------------------------------------------------------------------------
#
# An entry names the *whole* declaration site — repository-relative path, Go
# field name, JSON wire key, and the exact declared float type — and all four
# must match. Nothing coarser is permitted: a new float money field in the same
# file, a different float type on the same field, or the same field name with
# its json tag removed are each still reported. A waiver keyed on a path, a
# package, or a rule class would be a silent regression of the guard wearing a
# comment, and is the failure mode ADR 0012's allowlist is written to avoid.
#
# Entry 1 — market.OrderBookResponse.TickSize, at pkg/hstong/market/market.go:105
#
#     TickSize float64 `json:"spreadLevel"`
#
#   Why it is excepted. The field shipped in v0.1.0, so ADR 0011 guarantee 1
#   ("no existing exported type signature is modified") covers it and
#   float64 -> json.Number is a breaking change for every v0.1.x consumer of
#   pkg/hstong/market. The Gateway documents this wire value as a plain
#   `double`, and the SDK only ever reads it: it is never re-emitted, never
#   summed, and never rounded, so the silent precision loss ADR 0008 exists to
#   prevent cannot be reached through it. The Go name was chosen for this field
#   when the guard judged Go names only, specifically so it would not be
#   flagged; the json-tag rule added 2026-09-26 closed that evasion, and this
#   waiver replaces the hidden exception with a dated, reviewable one.
#
#   When it is removed. At v1.0.0 (run task G1), which retires ADR 0011
#   guarantee 1 along with the v0.1.x deprecation path. The field becomes
#   json.Number and this entry is deleted outright, with nothing replacing it —
#   the waiver exists only to honour a compatibility promise that v1.0 retires,
#   so it must not outlive the promise. If the field change is deferred past
#   v1.0.0, the waiver stays and this entry must be re-dated, not left to rot.
Waiver = namedtuple("Waiver", "path field key gotype")

WAIVERS = (
    Waiver("pkg/hstong/market/market.go", "TickSize", "spreadLevel", "float64"),
)


def waiver_for(rel, field, key, gotype):
    """Return the Waiver covering this exact declaration site, or None.

    All four components must match the entry. ``rel`` is repository-relative
    with forward slashes, ``key`` is the JSON wire key (None when the field
    carries no usable json tag, which therefore never matches an entry that
    names one), and ``gotype`` is the declared type with any struct tag
    stripped, so only the bare ``float64`` is excepted and not ``*float64``,
    ``[]float64`` or ``float32``.
    """
    for w in WAIVERS:
        if (w.path, w.field, w.key, w.gotype) == (rel, field, key, gotype):
            return w
    return None


def iter_go_files(root):
    for dirpath, _dirnames, filenames in os.walk(root):
        for name in filenames:
            if name.endswith(".go") and not name.endswith("_test.go"):
                yield os.path.join(dirpath, name)


def is_money_name(name):
    return any(w.lower() in MONEY_TOKENS for w in WORDS.findall(name))


def wire_key(tag):
    """Return the JSON wire key declared in a struct tag, or None.

    ``tag`` is the backticked span of a field's struct tag, or None.  Returns
    None — never raises — for a missing, unterminated, or json-less tag, so a
    field with no usable tag is still judged on its Go name alone.  The name is
    the part before the first comma, per encoding/json's tag option syntax;
    ``json:"-"`` yields "-", which tokenises to nothing and so never matches a
    money token.
    """
    if not tag:
        return None
    match = JSON_TAG.search(tag)
    if not match:
        return None
    return match.group(1).split(",", 1)[0].strip() or None


def find_violations(pkg_dir, root):
    """Return (violations, waived) for float money/quantity fields under ``pkg_dir``.

    A field fails when the Go field name *or* the JSON wire key is money-like;
    the two are the same predicate, so a wire key that reads ``spreadLevel``
    cannot hide behind a Go name like ``TickSize``.  Report lines name the Go
    field and the wire key together, since a reader who only sees the Go
    declaration cannot tell which of the two triggered the match.

    ``violations`` is what must be fixed.  ``waived`` is the same kind of report
    for a field an entry in ``WAIVERS`` covers, returned so ``main`` can print
    it: a suppression that leaves no trace in the build log is indistinguishable
    from a guard that was never run.
    """
    violations = []
    waived = []
    for path in sorted(iter_go_files(pkg_dir)):
        with open(path, encoding="utf-8") as fh:
            for lineno, line in enumerate(fh, 1):
                code = line.split("//", 1)[0]
                match = FIELD.match(code)
                if not match:
                    continue
                remainder = match.group(2).strip()
                bare = TAG.sub("", remainder).strip()
                if "=" in bare or not FLOAT.search(bare):
                    continue
                tag = TAG.search(remainder)
                key = wire_key(tag.group(0) if tag else None)
                for name in (n.strip() for n in match.group(1).split(",")):
                    if is_money_name(name):
                        matched = "field name"
                    elif key is not None and is_money_name(key):
                        matched = "wire key"
                    else:
                        continue
                    rel = os.path.relpath(path, root).replace(os.sep, "/")
                    where = 'wire key "%s"' % key if key else "no json tag"
                    # Collapse gofmt's column alignment so the report quotes the
                    # declaration as written rather than as padded.
                    decl = "%s %s" % (name, " ".join(remainder.split()))
                    report = "%s:%d: %s (%s; matched on %s)" % (
                        rel,
                        lineno,
                        decl,
                        where,
                        matched,
                    )
                    if waiver_for(rel, name, key, bare) is not None:
                        waived.append(report)
                    else:
                        violations.append(report)
    return violations, waived


def main():
    if not os.path.isdir(PKG_DIR):
        print("money-check OK: pkg/ does not exist yet")
        return 0

    violations, waived = find_violations(PKG_DIR, ROOT)

    # Waived fields are reported before the verdict, and reported even when the
    # verdict is a failure: a run that fails for one reason must not hide that
    # a second field is being let through on the strength of an ADR amendment.
    if waived:
        print(
            "money-check: %d declared exception(s) waived (ADR 0008, amended"
            " 2026-09-26); each expires at v1.0.0 when ADR 0011 guarantee 1 ends:"
            % len(waived)
        )
        for w in waived:
            print("  " + w)

    if violations:
        print(
            "money-check: float money/quantity fields found in pkg/ (ADR 0008):",
            file=sys.stderr,
        )
        for v in violations:
            print("  " + v, file=sys.stderr)
        print(
            "  use string or json.Number for money and quantities, or change the"
            " wire key if the value is not one",
            file=sys.stderr,
        )
        return 1

    print("money-check OK: no float money/quantity fields in pkg/")
    return 0


if __name__ == "__main__":
    sys.exit(main())
