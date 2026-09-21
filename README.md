# Pcom - private social network

Private as in the content is not public by default and discovery requires a human touch. Please refer to [manifesto](cmd/web/client/articles/why.md)
for more details.

Pcom uses [gogo](https://github.com/can3p/gogo) to handle forms and some other things!

If you want to follow the development, there is a [youtube playlist](https://www.youtube.com/playlist?list=PLa5K-kCUS-FozB6Cw7rJLFJaxyZd-MPpi) with demos!

## Official client

Official client is [blg](https://github.com/can3p/blg), command line client that plays well with pcom. See [docs/api.md](docs/api.md) for the API.

## Dependencies

* Go (version from `go.mod`)
* Node.js (version from `.tool-versions`) and yarn
* PostgreSQL
* libvips (`brew install vips pkg-config`)
* Docker, for the test suite (tests start their own Postgres container)
* `envsubst` (`brew install gettext`), used by `./generate.sh`

## Dev Setup

* Install go, asdf, postgres, watchexec
* `asdf install` (installs node)
* `npm install -g yarn`
* `cd cmd/web; yarn install`
* `go install github.com/rubenv/sql-migrate/sql-migrate@latest`
* `go install github.com/volatiletech/sqlboiler/v4@latest`
* `go install github.com/volatiletech/sqlboiler/v4/drivers/sqlboiler-psql@latest`
* `createuser pcom -W` use `pcom` as a password there
* `createdb --owner=pcom pcom_dev`
* `echo 'SESSION_SALT=random' >> cmd/web/.env`
* `echo 'SITE_ROOT=http://localhost:8080' >> cmd/web/.env`
* `echo 'DATABASE_URL=postgres://pcom:pcom@localhost:5432/pcom_dev?sslmode=disable' >> cmd/web/.env`
* `./sqlmigrate.sh up`

The server listens on `$PORT`, 8080 by default, so `SITE_ROOT` has to match it.
Registration is controlled by `system_settings.registration_open`. For local
development you can bypass it with `go run . -force-signup`.

### Run the app

```
cd cmd/web
yarn watch          # in one tab: rebuilds frontend assets into cmd/web/dist
make watchexec      # in another tab: restarts the server on changes
```

The server needs `cmd/web/dist/manifest.json` to exist, so run `yarn watch`
(or `yarn build`) at least once before starting it.

### Tests

```
make check   # build + all tests, no artifacts
make test    # tests only
make lint    # golangci-lint
```

Docker must be running: tests that touch the database start a Postgres
container via `testcontainers/postgres`.

### psql access

```
psql -U pcom pcom_dev
```

### schema changes

```
./sqlmigrate.sh new migration_name
```

Edit the file given by sql-migrate

```
./sqlmigrate.sh up
./generate.sh
```

`./generate.sh` regenerates the sqlboiler models in `pkg/model/core` from the
database in `cmd/web/.env`.

## Initial Setup

1. change remote and push to the new repo
2. change flytoml to point to the new app pcom
3. create the app on fly `flyctl apps create pcom`
4. create db, set 4gb ram `fly postgres create -n pcomdb`
5. attach db to the app `flyctl postgres attach -a pcom pcomdb`
6. Set secrets:

   ```
   flyctl secrets set SESSION_SALT=<random string>
   flyctl secrets set SITE_ROOT=https://pcom.com
   flyctl secrets set MJ_APIKEY_PUBLIC=<public key from mailjet>
   flyctl secrets set MJ_APIKEY_PRIVATE=<private key from mailjet>
   # this one should include scheme
   flyctl secrets set USER_MEDIA_ENDPOINT=<endpoint>
   flyctl secrets set USER_MEDIA_BUCKET=<bucket>
   flyctl secrets set USER_MEDIA_KEY=<key>
   flyctl secrets set USER_MEDIA_REGION=<region>
   flyctl secrets set USER_MEDIA_SECRET=<secret>
   flyctl secrets set SENDER_ADDRESS=<address>
   flyctl secrets set ADMIN_ADDRESS=<address>
   flyctl secrets set STATIC_CDN=<address> # in case you want to put static resources behind the cdn
   flyctl secrets set USER_MEDIA_CDN=<address> # in case you want to put user images behind the cdn
   flyctl secrets set ENABLE_PPROF=true # optional, serves pprof on :8081 (see `make pprof_tunnel`)
   ```

   The app switches to production behavior (mailjet sender, S3 storage,
   secure cookies, HSTS) when `FLY_APP_NAME` is set, which fly does
   automatically.
7. Do first deploy `fly deploy`, make sure you can reach the app via <appname>.fly.dev
8. Create a cert for your custom domain `fly certs add pcom.com`
9. After it screams at you, add required A and AAAA records
10. You might need to run `fly certs check pcom.com` a couple of times, `fly certs list` should show your domain with the status `ready`.
11. You should be able to reach your app via custom domain at this point
12. Go to mailjet and add new domain
13. Add sender email address there
14. Add required txt record to validate domain
15. Add required txt records to add DKIM and SPF settings
16. Run the following from the project root to get the database schema in place

Tab 1:

```
fly proxy 5433:5432 -a pcomdb   # or: make tunnel
```

Tab 2:

```
./run.sh             # opens a shell with the production secrets loaded
./sqlmigrate.sh up
```

`run.sh` evaluates `./env.pl`, which reads the app's secrets from fly and
rewrites `DATABASE_URL` to point at the proxied `localhost:5433`. Don't redirect
`./env.pl` into `cmd/web/.env`, because that would overwrite your local
development settings with production ones.

## Operational notes

* App instance is running in a wrong dc:

  ```
  fly scale count 0
  fly scale count 1 --region ams
  ```

## Modernization

Work to add test coverage and then modernize the codebase is planned in
[docs/implementation-plan.md](docs/implementation-plan.md).

## Credits

The project has been generated by [gogo-cli](https://github.com/can3p/gogo-cli) and uses [gogo](https://github.com/can3p/gogo) library
