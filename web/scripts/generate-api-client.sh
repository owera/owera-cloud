#!/usr/bin/env bash
# Regenerate the typed OpenAPI surface used by web/lib/api-client.ts.
#
# Source of truth: api/openapi.yaml in this repo. WS-14 evolves that file;
# we pin a copy at web/openapi.snapshot.yaml so dashboard work isn't whip-
# sawed mid-task. To refresh against latest:
#
#   ./web/scripts/generate-api-client.sh --refresh-snapshot
#
# Default run regenerates web/lib/api/generated.ts from the pinned snapshot.

set -euo pipefail

cd "$(dirname "$0")/.."

REFRESH=0
if [[ "${1:-}" == "--refresh-snapshot" ]]; then
  REFRESH=1
fi

SRC="../api/openapi.yaml"
SNAPSHOT="openapi.snapshot.yaml"
OUT="lib/api/generated.ts"

if [[ $REFRESH -eq 1 ]]; then
  if [[ ! -f "$SRC" ]]; then
    echo "error: $SRC not found (run from repo root web/)" >&2
    exit 1
  fi
  cp "$SRC" "$SNAPSHOT"
  echo "refreshed $SNAPSHOT from $SRC"
fi

if [[ ! -f "$SNAPSHOT" ]]; then
  echo "error: $SNAPSHOT missing; run with --refresh-snapshot to seed" >&2
  exit 1
fi

mkdir -p "$(dirname "$OUT")"
npx openapi-typescript "$SNAPSHOT" --output "$OUT"
echo "wrote $OUT"
