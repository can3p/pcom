#!/bin/sh
# Prints where the local dev services answer. The ports follow the overrides
# docker-compose.yml honours (PCOM_APP_PORT and friends).
#
#   tools/dev-urls.sh          print the addresses of postgres and tommy
#   tools/dev-urls.sh --app    also the app; run next to `docker compose up`
#                              (make dev), it waits until the app answers so the
#                              addresses show up below the startup logs
set -u

app_port=${PCOM_APP_PORT:-8080}
tommy_port=${PCOM_TOMMY_PORT:-8811}
s3_port=${PCOM_S3_PORT:-9555}
pg_port=${PCOM_PG_PORT:-5442}

print_urls() {
	echo
	echo "================================ pcom dev ================================"
	if [ "${1:-}" = app ]; then
		echo "  app        http://localhost:$app_port  (alice@example.test / password after make seed)"
	fi
	echo "  tommy      http://localhost:$tommy_port/ui/  (captured mail, S3 objects)"
	echo "  S3         http://localhost:$s3_port  (bucket pcom-media)"
	echo "  postgres   postgres://pcom:pcom@localhost:$pg_port/pcom"
	echo "=========================================================================="
	echo
}

if [ "${1:-}" != --app ]; then
	print_urls
	exit 0
fi

# The parent is the shell running `docker compose up`; stop waiting once it is
# gone (Ctrl-C, or compose failed to start).
parent=$PPID
if command -v curl >/dev/null 2>&1; then
	until curl -s -o /dev/null --max-time 2 "http://localhost:$app_port/"; do
		kill -0 "$parent" 2>/dev/null || exit 0
		sleep 2
	done
fi
print_urls app
