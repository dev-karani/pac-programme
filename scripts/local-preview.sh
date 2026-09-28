#!/usr/bin/env bash
set -euo pipefail
PAC_PREVIEW_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PAC_PREVIEW_DATA="$PAC_PREVIEW_ROOT/server/data"
PAC_PREVIEW_GOOSE="${PAC_GOOSE_BIN:-goose}"
if ! command -v "$PAC_PREVIEW_GOOSE" >/dev/null 2>&1; then
  PAC_PREVIEW_GOOSE="$HOME/go/bin/goose"
fi
for command in postgres pg_ctl pg_isready initdb createdb psql go npm; do
  command -v "$command" >/dev/null || { echo "Missing prerequisite: $command" >&2; exit 1; }
done
test -x "$PAC_PREVIEW_GOOSE" || command -v "$PAC_PREVIEW_GOOSE" >/dev/null
mkdir -p "$PAC_PREVIEW_DATA"
if [ ! -f "$PAC_PREVIEW_DATA/local-postgres/PG_VERSION" ]; then
  initdb -D "$PAC_PREVIEW_DATA/local-postgres" -A trust --no-locale -E UTF8
fi
if ! pg_isready -h 127.0.0.1 -p 55433 >/dev/null 2>&1; then
  pg_ctl -D "$PAC_PREVIEW_DATA/local-postgres" -l "$PAC_PREVIEW_DATA/postgres.log" -o "-p 55433 -h 127.0.0.1 -k '$PAC_PREVIEW_DATA'" start
fi
PAC_RUNNING_DIR="$(psql -h 127.0.0.1 -p 55433 -d postgres -tAc 'SHOW data_directory')"
if [ "$PAC_RUNNING_DIR" != "$PAC_PREVIEW_DATA/local-postgres" ]; then
  echo "Port 55433 belongs to another database. Stop here to protect its data." >&2
  exit 1
fi
if ! psql -h 127.0.0.1 -p 55433 -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='pac_demo'" | grep -qx 1; then
  createdb -h 127.0.0.1 -p 55433 pac_demo
fi
export DATABASE_URL="postgres://$(id -un)@127.0.0.1:55433/pac_demo?sslmode=disable"
export UPLOAD_DIR="$PAC_PREVIEW_DATA/uploads"
export WEB_DIR="$PAC_PREVIEW_ROOT/web/dist"
export APP_ADDR="127.0.0.1:8094"
cd "$PAC_PREVIEW_ROOT/server"
"$PAC_PREVIEW_GOOSE" -dir migrations postgres "$DATABASE_URL" up
if [ "${1:-}" = "--seed" ]; then go run ./cmd/pac seed; fi
npm --prefix "$PAC_PREVIEW_ROOT/web" run build
echo "PAC local demo: http://127.0.0.1:8094 (Ctrl+C stops the application; data is retained)."
exec go run ./cmd/pac
