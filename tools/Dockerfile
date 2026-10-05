# Dev tooling image for pcom: see docker-compose.yml (services `tools`, and
# `app`/`assets` later). Build context is the repository root, but nothing is
# copied in: the repository is mounted at /src at run time.
#
# sql-migrate is installed here, pinned by the ARG below, and never appears in
# go.mod. SQL_MIGRATE_VERSION must equal the one in the production Dockerfile:
# CI's lint job checks it.

# --- tools: migrations, psql, seed ---------------------------------
FROM golang:1.26-alpine AS tools

# psql/pg_dump for `make psql` and dumps; bash for the scripts.
RUN apk add --no-cache bash postgresql16-client

ARG SQL_MIGRATE_VERSION=v1.8.1
RUN GOBIN=/usr/local/bin go install github.com/rubenv/sql-migrate/sql-migrate@${SQL_MIGRATE_VERSION} \
 && rm -rf /root/go /root/.cache

# The container runs as the host UID/GID (compose `user:`), which has no
# passwd entry and no home, so every cache lives at a fixed, world-writable
# path. Compose mounts named volumes over /cache/gomod and /cache/gobuild; a
# new volume inherits these permissions.
ENV HOME=/tmp \
    GOMODCACHE=/cache/gomod \
    GOCACHE=/cache/gobuild \
    GOFLAGS=-buildvcs=false \
    PCOM_TOOLS_CONTAINER=1
RUN mkdir -p /cache/gomod /cache/gobuild /src \
 && chmod 1777 /cache/gomod /cache/gobuild

WORKDIR /src
CMD ["bash"]

# --- dev: the app with live reload (compose `app`/`assets`, task S4) --------
FROM tools AS dev

# vips-dev, gcc and musl-dev for the cgo build of govips; nodejs and yarn for
# the frontend bundle; watchexec to rebuild and restart on change.
RUN apk add --no-cache vips-dev gcc musl-dev nodejs yarn watchexec
ENV CGO_ENABLED=1

# Compose mounts a named volume over node_modules (the host copy may hold
# macOS binaries). A new volume copies this directory's owner and mode, so make
# it writable for whatever host UID the container runs as.
RUN mkdir -p /src/cmd/web/node_modules && chmod 1777 /src/cmd/web/node_modules
