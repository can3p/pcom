#!/usr/bin/env bash
# Runs sql-migrate (installed in the tools image, see tools/Dockerfile) with the
# given arguments. dbconfig.yml reads the database from DATABASE_URL.
#
# Runs inside the tools container: use `make migrate`, `make migrate-status`,
# `make migrate-down`, `make migration name=...` or `make migrate-prod`.
# PCOM_ALLOW_HOST_TOOLS=1 lets it run on the host (with DATABASE_URL set).
set -euo pipefail

if [ -z "${PCOM_TOOLS_CONTAINER:-}" ] && [ "${PCOM_ALLOW_HOST_TOOLS:-}" != 1 ]; then
  echo "sqlmigrate.sh runs in the tools container: use make migrate (or set PCOM_ALLOW_HOST_TOOLS=1)" >&2
  exit 1
fi
: "${DATABASE_URL:?DATABASE_URL must be set}"

cd "$(dirname "$0")"
exec sql-migrate "$@"
