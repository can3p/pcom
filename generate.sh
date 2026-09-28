#!/usr/bin/env bash
# Regenerates the sqlboiler models in pkg/model/core from the database in
# DATABASE_URL. sqlboiler and its psql driver are pinned in go.mod.
#
# Runs inside the tools container: use `make generate`.
# PCOM_ALLOW_HOST_TOOLS=1 lets it run on the host (with DATABASE_URL set).
set -euo pipefail

if [ -z "${PCOM_TOOLS_CONTAINER:-}" ] && [ "${PCOM_ALLOW_HOST_TOOLS:-}" != 1 ]; then
  echo "generate.sh runs in the tools container: use make generate (or set PCOM_ALLOW_HOST_TOOLS=1)" >&2
  exit 1
fi
: "${DATABASE_URL:?DATABASE_URL must be set}"

cd "$(dirname "$0")"

# sqlboiler's psql driver takes no DSN, so this is the one place that splits
# DATABASE_URL (postgres://user[:password]@host[:port]/dbname[?params]).
re='^postgres(ql)?://([^:@/]+)(:([^@]*))?@([^:/?]+)(:([0-9]+))?/([^?]+)'
if [[ ! "$DATABASE_URL" =~ $re ]]; then
  echo "generate.sh: cannot parse DATABASE_URL (want postgres://user:password@host:port/dbname)" >&2
  exit 1
fi
export POSTGRES_USER="${BASH_REMATCH[2]}"
export POSTGRES_PASSWORD="${BASH_REMATCH[4]}"
export POSTGRES_HOST="${BASH_REMATCH[5]}"
export POSTGRES_PORT="${BASH_REMATCH[7]:-5432}"
export POSTGRES_DB="${BASH_REMATCH[8]}"

config=$(mktemp --suffix=.toml 2>/dev/null || mktemp)
trap 'rm -f "$config"' EXIT
envsubst < sqlboiler.toml > "$config"

# sqlboiler finds its driver by path; `go tool -n` builds it if needed and
# prints where it is.
driver=$(go tool -n sqlboiler-psql)
go tool sqlboiler --add-panic-variants --no-hooks --no-tests --add-enum-types -c "$config" "$driver"
