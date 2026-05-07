#!/usr/bin/env bash
set -Eeuo pipefail

log() {
        printf '[init] %s\n' "$*"
}

die() {
        printf '[init] ERROR: %s\n' "$*" >&2
        exit 1
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

log "Running initialization script..."

INIT_SH="$SCRIPT_DIR/run.sh"

[[ -x "$INIT_SH" || -f "$INIT_SH" ]] || die "Missing init script: $INIT_SH"

bash "$INIT_SH"