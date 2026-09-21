# Copyright 2026 shing1211
# SPDX-License-Identifier: Apache-2.0
"""MkDocs hooks for the hstongapi4go documentation site.

`docs/SPEC.md` (owned elsewhere) links to repository files that live outside
`docs_dir`, such as `../proto/PROVENANCE.md`, and `docs/testing.md` links
`../test/integration/README.md`. MkDocs cannot resolve a target outside the
documentation directory, and `mkdocs build --strict` therefore fails. This hook
rewrites only those out-of-tree repository links to their GitHub URLs, so every
in-docs link is still validated by strict mode.
"""

import re

_REPO_BLOB = "https://github.com/shing1211/hstongapi4go/blob/main/"

# Matches "](\../proto/<path>[#anchor])" and "](\../test/<path>[#anchor])".
_OUT_OF_TREE = re.compile(r"\]\(\.\./((?:proto|test)/[^)\s#]+)(#[^)\s]+)?\)")


def on_page_markdown(markdown, *, page, config, files):
    """Rewrite out-of-tree repository links to GitHub URLs."""
    return _OUT_OF_TREE.sub(
        lambda m: "](" + _REPO_BLOB + m.group(1) + (m.group(2) or "") + ")",
        markdown,
    )
