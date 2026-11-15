#!/bin/sh
# Prints the directory of every tracked Go module, one per line.
# Usage: scripts/modules.sh [--libs]   (--libs skips examples/)
set -eu
cd "$(dirname "$0")/.."
git ls-files -- 'go.mod' '*/go.mod' | sed 's|/\{0,1\}go.mod$||; s|^$|.|' | sort | {
	if [ "${1:-}" = "--libs" ]; then grep -v '^examples' || true; else cat; fi
}
