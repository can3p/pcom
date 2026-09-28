# Pcom - private social network

Private as in the content is not public by default and discovery requires a human touch. Please refer to [manifesto](cmd/web/client/articles/why.md)
for more details.

Pcom uses [gogo](https://github.com/can3p/gogo) to handle forms and some other things!

If you want to follow the development, there is a [youtube playlist](https://www.youtube.com/playlist?list=PLa5K-kCUS-FozB6Cw7rJLFJaxyZd-MPpi) with demos!

## Official client

Official client is [blg](https://github.com/can3p/blg), command line client that plays well with pcom. See [docs/api.md](docs/api.md) for the API.

## Local development

Docker is the only dependency. Cold start, seeded users, ports, everyday commands and troubleshooting are in
[docs/running.md](docs/running.md):

```
cp .env.example cmd/web/.env
make dev-up && make migrate && make seed
make dev        # app and asset watcher in containers, http://localhost:8080
```

### Tests

```
make check   # build + all tests, no artifacts
make test    # tests only
make lint    # golangci-lint
```

Docker must be running: tests that touch the database start a Postgres
container via `pkg/testutil/testdb`.

### Browser tests

User flows (forms, htmx swaps, Stimulus controllers) are tested in a real
Chromium driven by [playwright-go](https://github.com/mxschmitt/playwright-go).
They live in `e2e/browser` behind the build tag `browser`, so `make test` and
`make check` don't run them.

```
make ui-deps    # once: installs the Playwright driver and Chromium
make test-ui    # builds the frontend, then runs the suite
make test-ui RUN=TestSmoke COUNT=3                  # narrow it, repeat it
make test-ui HEADED=1 SLOWMO=250 RUN=TestSmoke_LoginAndBoostedNavigation
make ui-trace F=.ui-artifacts/<Test>.trace.zip      # inspect a failure
```

Chromium runs **headless** by default, so no window opens and a passing run
prints only `ok: test-ui`. Add `HEADED=1` to see the browser and `SLOWMO=<ms>`
to slow each step down. Narrow the run with `RUN` when watching, because the
tests run in parallel and each one opens its own window. A failed test saves a
screenshot and a trace under `.ui-artifacts/` and prints their paths.
`docs/testing.md` explains how the suite works and how to write a test.

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
make tunnel          # fly proxy 5433 -a pcomdb
```

Tab 2:

```
make migrate-prod    # asks for confirmation first
```

`make migrate-prod` runs the migrations from the tools container against the
tunnel, with `DATABASE_URL` taken from `./env.pl` (the app's secrets on fly).
On Linux the tunnel must listen on an address the container can reach
(`flyctl proxy --bind-addr`). See [docs/running.md](docs/running.md).

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
