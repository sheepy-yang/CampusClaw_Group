#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PATTERN='-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----|AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{30,}|sk-[A-Za-z0-9_-]{20,}|(API_KEY|SECRET|PASSWORD|TOKEN)[[:space:]]*[:=][[:space:]]*["'"'"'][^"'"'"']{8,}["'"'"']'

set +e
matches="$(
  rg --files-with-matches --hidden --no-messages \
    --glob '!.git/**' \
    --glob '!node_modules/**' \
    --glob '!dist/**' \
    --glob '!.env' \
    --glob '!.env.*' \
    --glob '!package-lock.json' \
    -- "$PATTERN" .
)"
scan_status=$?
set -e

if (( scan_status > 1 )); then
  echo "Secret scan failed to execute (ripgrep exit ${scan_status})." >&2
  exit "$scan_status"
fi

if [[ -n "$matches" ]]; then
  echo "Potential secret material found in:"
  printf '%s\n' "$matches"
  exit 1
fi

echo "secret scan passed"
