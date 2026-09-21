#!/usr/bin/env python3
# Copyright 2026 shing1211
# SPDX-License-Identifier: Apache-2.0
"""Check that every relative Markdown link resolves to a file on disk.

Walks every ``*.md`` file in the repository (skipping generated, vendored, and
build-output directories) and fails when a relative link target does not exist.

Ignored link forms:

- absolute URLs with a scheme (``http://``, ``https://``, ``mailto:``, ...);
- protocol-relative links (``//host/...``);
- pure fragment links (``#anchor``);
- site-absolute paths (``/path``), which Markdown treats as relative to the site
  root rather than the repository.

Fenced code blocks are stripped before scanning so example snippets cannot
produce false positives.

Exit status is 0 when every relative link resolves, 1 otherwise. Output is a
sorted list of ``path:line: unresolved link -> target`` lines.

The T40 translations task adds a ``scripts/check_i18n.py`` call to
``make docs-check`` next to this script; this file deliberately stays focused on
link resolution.
"""

from __future__ import annotations

import os
import re
import sys
from urllib.parse import unquote

# Repository root is the parent of this script's directory (scripts/).
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

# Directory names skipped entirely while walking for Markdown files.
EXCLUDE_DIRS = {
    ".git",
    ".venv",
    "__pycache__",
    "dist",
    "gen",
    "node_modules",
    "proto",
    "site",
}

# Inline links and images: [text](target) and ![alt](target).
LINK_RE = re.compile(r"!?\[[^\]]*\]\(([^)]+)\)")

# Fenced code block delimiters (``` or ~~~).
FENCE_RE = re.compile(r"^\s*(```|~~~)")

# A link target is an external/absolute URI when it has a scheme, e.g.
# "https:", "mailto:", "tel:", "data:". Matches a leading RFC-3986 scheme.
SCHEME_RE = re.compile(r"^[a-zA-Z][a-zA-Z0-9+.\-]*:")

# Strips an optional Markdown title after the path: (path "Title").
TITLE_RE = re.compile(r"""^(\S+)(?:\s+["'].*)?$""")


def iter_markdown_files():
    """Yield absolute paths of every Markdown file under the repository root."""
    for root, dirs, files in os.walk(REPO_ROOT):
        dirs[:] = sorted(d for d in dirs if d not in EXCLUDE_DIRS)
        for name in files:
            if name.endswith(".md"):
                yield os.path.join(root, name)


def strip_fences(lines):
    """Blank out lines inside fenced code blocks so snippets are not scanned."""
    in_fence = False
    out = []
    for line in lines:
        if FENCE_RE.match(line):
            in_fence = not in_fence
            out.append("")
            continue
        out.append("" if in_fence else line)
    return out


def extract_target(raw):
    """Return the link target, or None when it is not a relative path link."""
    target = raw.strip()
    if target.startswith("<") and target.endswith(">"):
        target = target[1:-1].strip()

    match = TITLE_RE.match(target)
    if match:
        target = match.group(1)

    if not target or target.startswith("#"):
        return None
    if target.startswith("//") or target.startswith("/"):
        return None
    if SCHEME_RE.match(target):
        return None

    # Drop any fragment/query before resolving the path.
    path = target.split("#", 1)[0].split("?", 1)[0]
    if not path:
        return None
    return unquote(path)


def check_file(path):
    """Return a list of (line_number, raw_link, resolved_target) failures."""
    failures = []
    with open(path, "r", encoding="utf-8") as handle:
        lines = strip_fences(handle.read().splitlines())

    for lineno, line in enumerate(lines, start=1):
        for raw in LINK_RE.findall(line):
            target = extract_target(raw)
            if target is None:
                continue
            resolved = os.path.normpath(
                os.path.join(os.path.dirname(path), target)
            )
            if not os.path.exists(resolved):
                failures.append((lineno, raw.strip(), target))
    return failures


def main():
    markdown_files = sorted(iter_markdown_files())
    all_failures = []
    for path in markdown_files:
        for lineno, raw, target in check_file(path):
            rel = os.path.relpath(path, REPO_ROOT).replace(os.sep, "/")
            all_failures.append((rel, lineno, raw, target))

    for rel, lineno, raw, target in sorted(all_failures):
        print("%s:%d: unresolved link %s -> %s" % (rel, lineno, raw, target))

    print(
        "check_links: scanned %d Markdown files, %d unresolved link(s)"
        % (len(markdown_files), len(all_failures))
    )
    return 1 if all_failures else 0


if __name__ == "__main__":
    sys.exit(main())
