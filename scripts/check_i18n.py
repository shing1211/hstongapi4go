#!/usr/bin/env python3
# Copyright 2026 shing1211
# SPDX-License-Identifier: Apache-2.0
"""check_i18n.py - keep the six README translations in lockstep.

The English ``README.md`` is canonical; the five translations
(``README.zh-Hans.md``, ``README.zh-Hant.md``, ``README.ja.md``,
``README.ko.md``, ``README.es.md``) must stay structurally and administratively
consistent with it. This checker is dependency-free (stdlib only) and enforces:

1. Existence. Every expected README file exists.
2. Language switcher. Every README carries the canonical switcher line, which
   links all six READMEs in the fixed `` · ``-separated order.
3. Sync banner. Every README has ``Last synced: <date>`` and the date matches
   the canonical ``README.md`` exactly (the canonical file is the clock).
4. Heading drift. Every translation has the same number of Markdown headings
   (outside fenced code blocks) as English, within ``HEADING_TOLERANCE``.
   The tolerance is deliberately small: a translated section split into two
   headings, or a dropped section, is a structural change worth reviewing.
   Natural differences in wording do not change the heading count.
5. Actually translated. Each non-English README contains a locale-appropriate
   minimum of native-script / native-prose content, measured outside fenced
   code blocks. This catches a file that is a byte-for-byte English copy with
   only the filename changed.

Exit status is 0 when all files pass, 1 otherwise. Every violation is printed
as an actionable ``README.x.md: problem`` line. See TRANSLATING.md for the
workflow.

Run directly:  python scripts/check_i18n.py
"""

from __future__ import annotations

import os
import re
import sys
from typing import Dict, List, Optional, Tuple

# Repository root is the parent of this script's directory (scripts/).
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

CANONICAL = "README.md"

# locale -> (native label, filename, code?). Order is fixed; the switcher is
# built in exactly this order and must match in every file.
LANGUAGES = [
    ("en", "English", "README.md"),
    ("zh-Hans", "简体中文", "README.zh-Hans.md"),
    ("zh-Hant", "繁體中文", "README.zh-Hant.md"),
    ("ja", "日本語", "README.ja.md"),
    ("ko", "한국어", "README.ko.md"),
    ("es", "Español", "README.es.md"),
]

# Allowed absolute difference in heading count versus the canonical README.
HEADING_TOLERANCE = 2

# locale -> (compiled native-content pattern, minimum matches in prose).
# Counts are taken with fenced code blocks removed, so only translated prose is
# measured. Thresholds are far below what any real translation produces; they
# exist only to catch an untranslated English copy.
NATIVE_CONTENT: Dict[str, Tuple[re.Pattern, int]] = {
    "zh-Hans": (re.compile(r"[\u4e00-\u9fff]"), 200),
    "zh-Hant": (re.compile(r"[\u4e00-\u9fff]"), 200),
    "ja": (re.compile(r"[\u3040-\u30ff]"), 100),
    "ko": (re.compile(r"[\uac00-\ud7a3]"), 100),
    "es": (
        re.compile(
            r"[áéíóúüñ¿¡]"
            r"|\b(el|los|las|una|para|con|que|por|del|se|su|al|es|como|más|sin|sobre)\b",
            re.IGNORECASE,
        ),
        50,
    ),
}

# A line made of exactly six README links joined by " · ".
SWITCHER_LINE_RE = re.compile(
    r"^(\[[^\]]+\]\(\./README[^)]*\)(?: · \[[^\]]+\]\(\./README[^)]*\)){5})$",
    re.M,
)
HEADING_RE = re.compile(r"^(#{1,6})\s")
SYNC_RE = re.compile(r"Last synced:\s*(\S+)")
FENCE_RE = re.compile(r"^\s*(```|~~~)")


def expected_switcher() -> str:
    """Return the canonical switcher line shared by all six READMEs."""
    return " · ".join(f"[{label}](./{filename})" for _loc, label, filename in LANGUAGES)


def read(filename: str) -> str:
    """Read a repository-relative text file as UTF-8."""
    with open(os.path.join(ROOT, filename), encoding="utf-8") as handle:
        return handle.read()


def strip_fences(text: str) -> str:
    """Return the text with fenced code blocks blanked out."""
    out: List[str] = []
    in_fence = False
    for line in text.splitlines():
        if FENCE_RE.match(line):
            in_fence = not in_fence
            out.append("")
            continue
        out.append("" if in_fence else line)
    return "\n".join(out)


def heading_count(text: str) -> int:
    """Count Markdown headings outside fenced code blocks."""
    count = 0
    in_fence = False
    for line in text.splitlines():
        if FENCE_RE.match(line):
            in_fence = not in_fence
            continue
        if not in_fence and HEADING_RE.match(line):
            count += 1
    return count


def main() -> int:
    failures: List[str] = []
    switcher = expected_switcher()

    base_headings: Optional[int] = None
    base_date: Optional[str] = None

    # Pass 1: existence + switcher + banner, and record the canonical facts.
    texts: Dict[str, str] = {}
    for locale, _label, filename in LANGUAGES:
        path = os.path.join(ROOT, filename)
        if not os.path.exists(path):
            failures.append(f"{filename}: missing file")
            continue

        text = read(filename)
        texts[filename] = text

        matched = SWITCHER_LINE_RE.search(text)
        if matched is None or matched.group(1) != switcher:
            failures.append(
                f"{filename}: language switcher missing or wrong; expected the "
                f"line linking all six READMEs: {switcher}"
            )

        sync = SYNC_RE.search(text)
        if sync is None:
            failures.append(f"{filename}: missing 'Last synced: <date>' banner")
        elif locale == "en":
            base_date = sync.group(1)
        elif base_date is not None and sync.group(1) != base_date:
            failures.append(
                f"{filename}: Last synced: {sync.group(1)} != canonical "
                f"{CANONICAL} {base_date}"
            )

        if locale == "en":
            base_headings = heading_count(text)

    if base_date is None and os.path.exists(os.path.join(ROOT, CANONICAL)):
        failures.append(f"{CANONICAL}: missing 'Last synced: <date>' banner")

    # Pass 2: heading drift + actually-translated guard (canonical known now).
    for locale, _label, filename in LANGUAGES:
        if filename not in texts:
            continue
        text = texts[filename]

        if locale != "en" and base_headings is not None:
            count = heading_count(text)
            if abs(count - base_headings) > HEADING_TOLERANCE:
                failures.append(
                    f"{filename}: heading count {count} differs from English "
                    f"{base_headings} by more than {HEADING_TOLERANCE} "
                    f"(structural drift; split/merged or missing sections?)"
                )

        if locale in NATIVE_CONTENT:
            pattern, minimum = NATIVE_CONTENT[locale]
            hits = len(pattern.findall(strip_fences(text)))
            if hits < minimum:
                failures.append(
                    f"{filename}: only {hits} native-content match(es) outside "
                    f"code blocks (need >= {minimum}); file may be untranslated"
                )

    if failures:
        print("i18n check failed:", file=sys.stderr)
        for item in failures:
            print(f"  {item}", file=sys.stderr)
        print(
            f"i18n: {len(failures)} violation(s); see TRANSLATING.md",
            file=sys.stderr,
        )
        return 1

    print(f"i18n OK: {len(LANGUAGES)} languages consistent")
    return 0


if __name__ == "__main__":
    sys.exit(main())
