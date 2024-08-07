#!/usr/bin/env bash

# quick hack - if we run under run.sh, all vars are in the env already
if [ -z "$DATABASE_URL" ]; then
  set -a; source cmd/web/.env; set +a
fi

FIELDS=$(echo $DATABASE_URL | awk '{n = split($0, arr, /[\/@:?]*/); for (i = 1; i <= n; ++i) { print arr[i] }}')
DATABASE_PROTO=$( echo $FIELDS | awk '{ print $1 }')
export POSTGRES_USER=$( echo $FIELDS | awk '{ print $2 }')
export POSTGRES_PASSWORD=$( echo $FIELDS | awk '{ print $3 }')
export POSTGRES_HOST=$( echo $FIELDS | awk '{ print $4 }')
export POSTGRES_PORT=$( echo $FIELDS | awk '{ print $5 }')
export POSTGRES_DB=$( echo $FIELDS | awk '{ print $6 }')

sql-migrate $@
