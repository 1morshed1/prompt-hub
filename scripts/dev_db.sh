#!/usr/bin/env bash
# dev_db.sh — spin up an isolated, throwaway PostgreSQL cluster for local development.
#
# Why this exists: PromptHub needs Postgres, but you may not want to install/configure a
# system service just to try it. This script runs a private cluster under ./.pgdata on port
# 5433, so it never collides with a system Postgres and can be deleted at any time.
#
# NOTE: PostgreSQL refuses to run as the root user. Run this as a normal user. In a container
# where you are root, create an unprivileged user first (see docs/learning/01-getting-started.md).
set -euo pipefail

# Find the Postgres binaries (Debian/Ubuntu put them under /usr/lib/postgresql/<ver>/bin).
if ! command -v pg_ctl >/dev/null 2>&1; then
  for d in /usr/lib/postgresql/*/bin; do
    [ -x "$d/pg_ctl" ] && export PATH="$PATH:$d"
  done
fi

PGDATA="${PROMPTHUB_PGDATA:-$(pwd)/.pgdata}"
PGPORT="${PROMPTHUB_PGPORT:-5433}"
PGHOST="${PROMPTHUB_PGHOST:-localhost}"
SOCKDIR="${PROMPTHUB_PGSOCK:-/tmp}"
DBNAME="prompthub"
LOGFILE="$PGDATA/server.log"

start() {
  if [ ! -d "$PGDATA/base" ]; then
    echo "initdb -> $PGDATA"
    initdb -D "$PGDATA" -U postgres --auth=trust >/dev/null
  fi
  if pg_ctl -D "$PGDATA" status >/dev/null 2>&1; then
    echo "already running"
  else
    pg_ctl -D "$PGDATA" -o "-p $PGPORT -k $SOCKDIR" -l "$LOGFILE" start
    sleep 1
  fi
  # Create the database if it doesn't exist yet.
  if ! psql -h "$PGHOST" -p "$PGPORT" -U postgres -lqt | cut -d'|' -f1 | grep -qw "$DBNAME"; then
    createdb -h "$PGHOST" -p "$PGPORT" -U postgres "$DBNAME"
    echo "created database '$DBNAME'"
  fi
  echo "DATABASE_URL=postgres://postgres@${PGHOST}:${PGPORT}/${DBNAME}?sslmode=disable"
}

case "${1:-}" in
  start)  start ;;
  stop)   pg_ctl -D "$PGDATA" stop ;;
  status) pg_ctl -D "$PGDATA" status ;;
  psql)   shift; psql -h "$PGHOST" -p "$PGPORT" -U postgres -d "$DBNAME" "$@" ;;
  url)    echo "postgres://postgres@${PGHOST}:${PGPORT}/${DBNAME}?sslmode=disable" ;;
  *) echo "usage: $0 {start|stop|status|psql|url}" >&2; exit 1 ;;
esac
