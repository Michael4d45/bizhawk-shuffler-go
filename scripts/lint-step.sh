#!/usr/bin/env sh
# Usage: lint-step.sh <label> <command> [args...]
set -eu
label=$1
shift
echo "==> $label"
start=$(date +%s)
"$@"
elapsed=$(($(date +%s) - start))
echo "==> $label finished in ${elapsed}s"
