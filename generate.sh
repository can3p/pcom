#!/usr/bin/env bash
# Regenerates the sqlboiler models in pkg/model/core from the migrations alone:
# creates a throwaway pcom_codegen database on the server in DATABASE_URL,
# applies every migration there, runs sqlboiler against it and drops it again
# (also on failure). The database in DATABASE_URL itself is never touched, so
# the models do not depend on its state. sqlboiler and its psql driver are
# installed in the tools image (tools/Dockerfile), at the version of the
# sqlboiler library in go.mod; the script refuses to run if the two differ,
# because generated code must match the library it runs against.
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

lib=$(go list -m -f '{{.Version}}' github.com/volatiletech/sqlboiler/v4)
gen=$(sqlboiler --version | awk '{print $NF}')
gen="v${gen#v}"
if [ "$lib" != "$gen" ]; then
  echo "generate.sh: sqlboiler $gen in the tools image, but go.mod has the library at $lib." >&2
  echo "Bump SQLBOILER_VERSION in tools/Dockerfile to $lib and rebuild (docker compose build tools)." >&2
  exit 1
fi

# sqlboiler's psql driver takes no DSN, so this is the one place that splits
# DATABASE_URL (postgres://user[:password]@host[:port]/dbname[?params]).
re='^postgres(ql)?://([^:@/]+)(:([^@]*))?@([^:/?]+)(:([0-9]+))?/([^?]+)'
if [[ ! "$DATABASE_URL" =~ $re ]]; then
  echo "generate.sh: cannot parse DATABASE_URL (want postgres://user:password@host:port/dbname)" >&2
  exit 1
fi
CODEGEN_DB=pcom_codegen
export POSTGRES_USER="${BASH_REMATCH[2]}"
export POSTGRES_PASSWORD="${BASH_REMATCH[4]}"
export POSTGRES_HOST="${BASH_REMATCH[5]}"
export POSTGRES_PORT="${BASH_REMATCH[7]:-5432}"
export POSTGRES_DB="$CODEGEN_DB"
admin_url="$DATABASE_URL"
codegen_url="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${CODEGEN_DB}?sslmode=disable"

tmpdir=$(mktemp -d)
config="$tmpdir/sqlboiler.toml"
drop_db() { psql "$admin_url" -qX -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $CODEGEN_DB WITH (FORCE)" >/dev/null; }
cleanup() { rc=$?; rm -rf "$tmpdir"; drop_db || rc=1; exit "$rc"; }
trap cleanup EXIT

drop_db
psql "$admin_url" -qX -v ON_ERROR_STOP=1 -c "CREATE DATABASE $CODEGEN_DB" >/dev/null
DATABASE_URL="$codegen_url" ./sqlmigrate.sh up
envsubst < sqlboiler.toml > "$config"

# sqlboiler finds its driver by path.
driver=$(command -v sqlboiler-psql)
sqlboiler --add-panic-variants --no-hooks --no-tests --add-enum-types -c "$config" "$driver"
