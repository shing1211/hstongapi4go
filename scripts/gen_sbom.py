#!/usr/bin/env python3
# Copyright 2026 shing1211
# SPDX-License-Identifier: Apache-2.0

"""
Generate SPDX SBOM for the hstongapi4go module.

Usage:
    python scripts/gen_sbom.py [output_file]

The script reads go.mod to extract module name, version, and dependencies,
then emits an SPDX 2.3 JSON document. No external SBOM tools required.

Output format: SPDX 2.3 JSON
"""

import json
import subprocess
import sys
import textwrap
from datetime import datetime, timezone
from pathlib import Path

SPDX_VERSION = "SPDX-2.3"
DATA_LICENSE = "Apache-2.0"
CREATOR_TOOL = "hstongapi4go/scripts/gen_sbom.py:v1.0"


def now_iso() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def parse_go_mod(go_mod_path: str) -> dict:
    """Parse go.mod and return module info + dependencies."""
    content = Path(go_mod_path).read_text(encoding="utf-8")
    module_name = None
    go_version = None
    require = []
    in_require = False

    for raw_line in content.splitlines():
        line = raw_line.strip()
        if line.startswith("module "):
            module_name = line.split("module ", 1)[1]
        elif line.startswith("go "):
            go_version = line.split("go ", 1)[1]
        elif line == "require (":
            in_require = True
        elif line == ")":
            in_require = False
        elif in_require and line and not line.startswith("//"):
            parts = line.split()
            if len(parts) >= 2:
                require.append({"name": parts[0], "version": parts[1]})
            elif len(parts) == 1:
                require.append({"name": parts[0]})
        elif not in_require and line and not line.startswith("//"):
            parts = line.split()
            if len(parts) >= 2 and parts[0] == "require":
                require.append({"name": parts[1], "version": parts[2] if len(parts) > 2 else "v0.0.0"})

    return {
        "module": module_name or "unknown",
        "go_version": go_version or "unknown",
        "dependencies": require,
    }


def spdx_pkg_ref(name: str) -> str:
    return f"SPDXRef-{name.replace('/', '-').replace('@', '-')}"


def build_sbom(module_info: dict, doc_uri: str) -> dict:
    doc_name = module_info["module"]
    packages = []
    relationships = []

    root_ref = spdx_pkg_ref(module_info["module"])
    packages.append({
        "SPDXID": root_ref,
        "name": module_info["module"],
        "versionInfo": "v0.0.0",
        "downloadLocation": f"git+https://github.com/shing1211/hstongapi4go",
        "filesAnalyzed": False,
        "supplier": "Organization: shing1211",
        "declaredLicense": "Apache-2.0",
    })
    relationships.append({
        "spdxElementId": "SPDXRef-DOCUMENT",
        "relationshipType": "DESCRIBES",
        "relatedSpdxElement": root_ref,
    })

    for dep in module_info["dependencies"]:
        dep_name = dep["name"]
        dep_version = dep.get("version", "None")
        dep_ref = spdx_pkg_ref(dep_name)
        packages.append({
            "SPDXID": dep_ref,
            "name": dep_name,
            "versionInfo": dep_version,
            "downloadLocation": f"https://pkg.go.dev/{dep_name}@{dep_version}",
            "filesAnalyzed": False,
        })
        relationships.append({
            "spdxElementId": root_ref,
            "relationshipType": "DEPENDENCY_OF",
            "relatedSpdxElement": dep_ref,
        })

    doc = {
        "spdxVersion": SPDX_VERSION,
        "dataLicense": DATA_LICENSE,
        "SPDXID": "SPDXRef-DOCUMENT",
        "name": doc_name,
        "documentNamespace": doc_uri,
        "creationInfo": {
            "created": now_iso(),
            "creators": [f"Tool: {CREATOR_TOOL}"],
        },
        "packages": packages,
        "relationships": relationships,
    }
    return doc


def main():
    output_path = sys.argv[1] if len(sys.argv) > 1 else None

    go_mod_path = Path(__file__).parent.parent / "go.mod"
    module_info = parse_go_mod(go_mod_path)

    doc_uri = f"https://github.com/shing1211/hstongapi4go/sbom/v1"
    sbom = build_sbom(module_info, doc_uri)

    json_str = json.dumps(sbom, indent=2, ensure_ascii=False)

    if output_path:
        Path(output_path).write_text(json_str + "\n", encoding="utf-8")
        print(f"SBOM written to {output_path}")
    else:
        print(json_str)


if __name__ == "__main__":
    main()
