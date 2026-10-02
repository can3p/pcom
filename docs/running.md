# Running pcom locally

Docker is the only dependency for development. Everything else (Go, Postgres client, sqlboiler, sql-migrate,
Node, libvips) lives in containers. `docker-compose.yml` is the source of truth for ports and services; use the
make targets instead of typing `docker compose` by hand.

## Cold start

```bash
cp .env.example cmd/web/.env
make dev-up && make migrate && make seed
```

If you already have a `cmd/web/.env` from the old setup, move it aside first: its `DATABASE_URL` probably points
at a Postgres installed on your machine (port 5432). The one from `.env.example` points at the compose Postgres
on `localhost:5442`.

`make dev-up` starts Postgres and tommy (mail sink and S3 stand-in), waits until they are healthy and prints
their addresses. `make migrate` applies the migrations, `make seed` fills the database with a small named world (see
[Seeding](#seeding)). The first run builds the tools image (sql-migrate, sqlboiler, psql), which is slow once.

Then pick one way to run the app.

### Container mode

```bash
make dev
```

Runs the app (live reload on `.go`, `.html` and `.md` changes) and the frontend watcher in containers, in the
foreground. The app starts once the frontend watcher has finished its first build, and when it answers,
`make dev` prints the addresses of the app, tommy, S3 and Postgres below the logs. Open http://localhost:8080
and log in as `alice@example.test` / `password`. `make dev-logs` follows the logs from another terminal;
`make dev-down` stops everything.

The containers set their in-network values (database host, `SITE_ROOT`, `PORT`) as real environment variables,
and the app never lets `cmd/web/.env` override a variable that is already set, so the same `.env` serves both
modes.

File watching: see [macOS](#macos) if edits don't restart the app.

### Host mode

Needs libvips and Node.js (version from `.tool-versions`) with yarn on the host, plus `watchexec`. Only this
mode needs them.

```bash
make dev-up                    # postgres and tommy, as above
cd cmd/web
yarn install                   # once
yarn watch                     # tab 1: rebuilds the frontend into cmd/web/dist
make watchexec                 # tab 2: restarts the server on changes
```

`cmd/web/dist/manifest.json` must exist before the server starts, so let `yarn watch` (or `yarn build`) finish
once first. The server listens on `$PORT` (8080), and `SITE_ROOT` has to match. Registration is controlled by
`system_settings.registration_open`; the seed opens it, otherwise run the app with `-force-signup`.

## Mail and uploads

tommy runs in the stack and is ready, but **the app doesn't use it yet**:

* Mail: outside production the app prints mail to its console (the `make dev` logs, or the terminal running
  `make watchexec`). tommy's inbox at http://localhost:8811/ui/ stays empty.
* Uploads: outside production the app stores them in `cmd/web/user_media/`. tommy's S3 bucket `pcom-media` on
  port 9555 stays empty.

Pointing the app at tommy needs code changes (S3 outside production with path-style addressing, and a
configurable Mailjet base URL), planned for wave R2. The values it will use are in `.env.example`, commented out.

## Seeding

`make seed` runs `go run ./cmd/web seed` in the `app` container (the dev image: building `cmd/web` needs cgo
and libvips, which the tools image lacks). It reads `DATABASE_URL` (and `SITE_ROOT`) and
prints a summary with the logins. It runs in one transaction, so a failure changes nothing.

| Command | Does | Use when |
|---|---|---|
| `make seed` | Adds the seed data. Exits if any users already exist. | First run on a fresh, migrated database. |
| `make seed-reset` | Truncates every table except `migrations` and `system_settings`, then seeds. | You want the seed world back after experimenting. |

Both refuse to run when `FLY_APP_NAME` is set (production guard). `seed-reset` throws away all data in the
database, not only earlier seed data.

### Seeded users

Every account logs in with the email and the password `password`.

| User | Login | Password | Profile visibility | Relationships |
|---|---|---|---|---|
| alice | alice@example.test | password | connections | connected to bob; second degree to carol; owns the API key; has a pending mediation request to carol |
| bob | bob@example.test | password | registered_users | connected to alice and carol |
| carol | carol@example.test | password | public | connected to bob; target of alice's mediation request |
| dave | dave@example.test | password | connections | unrelated to everyone |
| eve | eve@example.test | password | registered_users | has an unaccepted invite (eve-friend@example.test) |

This table mirrors the doc comment of `pkg/testutil/seed/seed.go`; when one changes, change the other.

### What else is seeded

* Alice's posts: one each of `direct_only`, `second_degree` and `public` visibility (published), plus a draft.
* Bob's post with a URL, `direct_only` ("Bob, link").
* A share link for Alice's public post.
* A comment thread three levels deep on Alice's public post (bob, alice, bob).
* An open prompt from Bob to Alice ("What are you reading?").
* An RSS feed on `https://example.test/seed/feed.xml` (title "Seed feed") with three items, subscribed by Alice.
  The poller fails on that URL harmlessly.
* Alice asks Bob to introduce her to Carol; nobody has decided yet.
* Registration is opened (`system_settings.registration_open`) in the seeded database only.

### Paging data

Every paged list has more than one page at the default page size (30), with nothing to create by hand.
Paging posts' subjects start with `Paging: `; all of it is spread over the last days, older than the world
above.

* Carol has 60 public posts: explore (logged in), the anonymous index and `/users/carol` each load more,
  and `/rss/public/carol` stops at the RSS limit (50).
* Bob left 30 comments on Alice's direct-only "Paging: Alice's busy thread".
* Alice follows "Paging feed" (`https://example.test/seed/paging.xml`) with 30 items in her feed.
* So Alice's `/feed` holds posts, comments and RSS items interleaved, over several pages.

To see paging on less data, run the app with a smaller page, for example `PAGE_SIZE=5` (and
`RSS_LIMIT` for the RSS outputs).

### Alice's API key

The key is fixed: `00000000-0000-4000-8000-000000000001`. The API takes it as a bearer token in the
`Authorization` header (`pkg/auth/auth.go`); see [api.md](api.md) for the endpoints. Use the same value as the
API key of the [blg](https://github.com/can3p/blg) client, pointed at http://localhost:8080.

```bash
curl -H 'Authorization: Bearer 00000000-0000-4000-8000-000000000001' http://localhost:8080/api/v1/posts
```

## Ports

Everything the stack publishes on the host. Each port can be overridden with the variable in the last column,
so a second checkout can run next to this one: `PCOM_PG_PORT=5443 PCOM_TOMMY_PORT=8812 ... docker compose -p other up -d`
(project names keep containers and volumes apart; no service sets `container_name`).

| Host port | What answers | In network | Override |
|---|---|---|---|
| 5442 | Postgres 16, db/user/password `pcom` | `postgres:5432` | `PCOM_PG_PORT` |
| 8811 | tommy UI (`/ui/`) and API (`/api/v1/`); captured mail; Mailjet v3.1 send API | `tommy:8811` | `PCOM_TOMMY_PORT` |
| 8822 | tommy fake vendor ingress | `tommy:8822` | `PCOM_TOMMY_INGRESS_PORT` |
| 9555 | tommy S3, path-style, any credentials, bucket `pcom-media` | `tommy:9000` | `PCOM_S3_PORT` |
| 8080 | the app (`make dev`, profile `dev`) | `app:8080` | `PCOM_APP_PORT` |

9000, 9090 and 9091 are avoided on purpose: MinIO, `make pprof_tunnel` and `make pprof_heap` use them. Note that
host mode and `.env.example` assume the default ports; if you override one, update `cmd/web/.env` too.

## Everyday commands

| Command | Does |
|---|---|
| `make migrate` | Apply pending migrations to the compose database. |
| `make migrate-status` | Show which migrations are applied. |
| `make migrate-down` | Roll back the last migration. |
| `make migration name=add_foo` | Create `migrations/<timestamp>-add_foo.sql`; edit it, then `make migrate`. |
| `make generate` | Regenerate `pkg/model/core` from the migrations alone, using a throwaway `pcom_codegen` database that is dropped afterwards. CI checks the models are current, so run it after every migration. |
| `make psql` | psql on the compose database. Pass arguments with `ARGS`: `make psql ARGS="-c 'select count(*) from users'"`. |
| `make tools-shell` | Bash in the tools container; run one command with `make tools-shell CMD='go version'`. |
| `make db-reset` | Drop, recreate and migrate the dev database. |
| `make dev-logs` | Follow the logs of the `dev` profile (app and assets). |
| `make dev-down` | Stop all compose services (data is kept). |
| `make migrate-prod` | Apply migrations to production. Asks for confirmation; needs `make tunnel` running in another terminal. |

## Resetting

| Command | Throws away | Keeps |
|---|---|---|
| `make seed-reset` | All rows except `migrations` and `system_settings`; then reseeds. | Schema, S3 objects, volumes. |
| `make db-reset` | The whole dev database (schema and data), recreated and migrated, empty. Run `make seed` after. | S3 objects in tommy, Go caches, node_modules. |
| `docker compose down -v` | Everything: database, tommy's buckets and objects, Go caches, `node_modules` volumes. The next `make seed` recompiles the seed command. | Files in the checkout. |

Use `docker compose --profile '*' down -v` if the `dev` profile containers exist.

## macOS

macOS with Docker Desktop is the primary development platform. All images (Postgres, tommy, the tools and dev
images built from `golang:1.26-alpine`) have arm64 variants, so nothing runs under emulation on Apple silicon.

* **File watching in container mode.** Docker Desktop's VirtioFS file sharing (the default) forwards change
  events through the bind mount, so saving a file restarts the app. If it doesn't (older Docker Desktop or gRPC
  FUSE file sharing), poll instead: `PCOM_WATCH_POLL=1000 make dev` checks every second.
* **`node_modules`.** The containers keep their own Linux `node_modules` in a volume; the macOS copy under
  `cmd/web/node_modules` (for host mode) is never used by them, and the other way round.
* **Host mode** needs `brew install vips pkg-config watchexec` and Node.js from `.tool-versions`.
* **`make migrate-prod`** reaches `make tunnel` on your Mac through `host.docker.internal`, which Docker Desktop
  provides; no extra flags are needed (on Linux the tunnel must bind an address the container can reach).

## Troubleshooting

* Postgres is on 5442, not 5432, so a native Postgres on 5432 cannot silently take the app's connections. Use
  `localhost:5442` from the host.
* Files written by the containers (migrations, generated models, `cmd/web/dist`) belong to your UID/GID: make
  passes `HOST_UID` and `HOST_GID` to compose. Run the make targets rather than `docker compose` directly, or
  set those variables yourself.
* The first make target builds the tools image; sql-migrate and sqlboiler are installed in it
  (`tools/Dockerfile`), not in `go.mod`. The first `make seed` builds the dev image and compiles `cmd/web`; the Go caches live in
  the `gomod` and `gobuild` volumes, which `docker compose down -v` clears.
* `make generate` says the sqlboiler versions differ: `go.mod` moved the sqlboiler library, so bump
  `SQLBOILER_VERSION` in `tools/Dockerfile` to match and run `docker compose build tools`.
* `make seed` says users already exist: use `make seed-reset`.
* A port is already in use: override it, see [Ports](#ports).
