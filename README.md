# Pcom - private social network

Private as in the content is not public by default and discovery requires a human touch. Please refer to [manifesto](cmd/web/client/articles/why.md)
for more details.

If you want to follow the development, there is a [youtube playlist](https://www.youtube.com/playlist?list=PLa5K-kCUS-FozB6Cw7rJLFJaxyZd-MPpi) with demos!

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
   ```
7. Do first deploy `fly deploy`, make sure you can reach the app via <appname>.fly.dev
8. Create a cert for your custom domain `fly certs add pcom.com`
9. After it screams at you, add required A and AAAA records
10. You might need to run `fly certs check pcom.com` a couple of times, `fly certs list` should show your domain with the status `ready`.
11. You should be able to reach your app via custom domain at this point
12. Got to mailjet and add new domain
13. Add sender email address there
14. Add required txt record to validate domain
15. Add required txt records to add DKIM and SPF settings
16. Add postgres db env var to `cmd/web/.env` via `./env.pl > cmd/web/.env`, remove `sslMode=disable` and replace domain name with localhost
18. Run the following from the project root to get the database schema in place and generate orm files

```
./sqlmigrate.sh
./generate.sh
```

## Development

```
cd cmd/web
yarn
yarn watch # in one tab
make watchexec # in another tab
```

## Credits

The project has been generated by [gogo-cli](https://github.com/can3p/gogo-cli) and uses [gogo](https://github.com/can3p/gogo) library
