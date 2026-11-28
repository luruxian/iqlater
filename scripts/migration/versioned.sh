#!/bin/bash

# Generate or check iqlater PostgreSQL versioned migrations (Atlas format).
# Usage: ./versioned.sh <diff|check> [name]
#
# Uses MIGRATE_DEV_URL when set (must be an empty database with search_path=public).
# Otherwise starts a throwaway postgres:18 container on a random local port and
# removes it on exit.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

usage() {
	echo "Usage: $0 <diff|check> [name]" >&2
	exit 2
}

mode="${1:-}"
name="${2:-}"

case "$mode" in
diff)
	[ -n "$name" ] || { echo "Error: name is required. Usage: make migrate-diff name=<name>" >&2; exit 2; }
	args=(-name "$name")
	;;
check)
	args=(-check)
	;;
*)
	usage
	;;
esac

dev_url="${MIGRATE_DEV_URL:-}"

if [ -z "$dev_url" ]; then
	container="iqlater-migrate-dev-$$"
	trap 'docker rm -f "$container" >/dev/null 2>&1 || true' EXIT

	docker run -d --rm --name "$container" \
		-e POSTGRES_PASSWORD=pass -e POSTGRES_DB=dev \
		-p 127.0.0.1::5432 postgres:18 >/dev/null

	# Probe over TCP: the image's init-time server only listens on the unix socket.
	for _ in $(seq 1 60); do
		if docker exec "$container" pg_isready -h 127.0.0.1 -U postgres -d dev >/dev/null 2>&1; then
			break
		fi
		sleep 1
	done
	docker exec "$container" pg_isready -h 127.0.0.1 -U postgres -d dev >/dev/null

	port="$(docker port "$container" 5432/tcp | head -n1 | awk -F: '{print $NF}')"
	dev_url="postgres://postgres:pass@127.0.0.1:${port}/dev?sslmode=disable&search_path=public"
fi

cd "$PROJECT_ROOT"
go run ./cmd/migrate-diff -dev-url "$dev_url" "${args[@]}"
