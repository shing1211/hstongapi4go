#!/usr/bin/env bash
# Copyright 2026 shing1211
# SPDX-License-Identifier: Apache-2.0
#
# proto_verify.sh — fail if the committed gen/ tree drifts from proto/.
#
# Regenerates the protobuf code into a scratch directory and diffs it against
# the committed gen/ tree. Used by `make proto-verify` in CI (Linux/bash); it
# never mutates the working tree, so a failed check leaves gen/ untouched.
#
# Requires: buf and protoc-gen-go on PATH (see `make tools`).
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

if ! command -v buf >/dev/null 2>&1; then
	echo "proto-verify: buf not found on PATH; install it with 'make tools'" >&2
	exit 1
fi

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT

# buf's --output is a *base* directory: it is prepended to the plugin's `out`
# (which is `gen` in buf.gen.yaml), so the fresh tree lands at "$scratch/gen".
echo "proto-verify: regenerating into $scratch/gen"
buf generate --output "$scratch"

if [ ! -d gen ]; then
	echo "proto-verify: gen/ is missing; run 'make proto' and commit the output" >&2
	exit 1
fi

if diff -ru gen "$scratch/gen"; then
	echo "proto-verify: gen/ matches proto/ (no drift)"
else
	echo "proto-verify: gen/ is stale — run 'make proto' and commit the result" >&2
	exit 1
fi
