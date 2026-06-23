#!/usr/bin/env bash
set -euo pipefail

# Repeatable local performance/concurrency smoke for PSC-RM-082.
# Synthetic lab-test data only. Writes to a caller-supplied --db path or a temp DB.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

exec go run ./cmd/project-scientist smoke performance "$@"
