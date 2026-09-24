#!/usr/bin/env bash
# seed.sh — thin wrapper around scripts/seed.py (which uses the Python SDK).
# The server must already be running. Usage: ./scripts/seed.sh [BASE_URL]
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
exec python3 "$here/seed.py" "${1:-http://localhost:8080}"
