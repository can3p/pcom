# Self-hosting

Pcom is open source, and you can run your own instance. A local copy needs only Docker and starts with a few commands, including sample users and posts.

[Running pcom locally](../running.md) walks through the cold start, seeded users, ports and troubleshooting. Settings for a real deployment, such as initial setup and operational notes, are in the [README](../../README.md). Every option is an environment variable or flag, listed by `go run ./cmd/web serve --help`.

Pages hold 30 items and RSS outputs 50 by default; change them with `--page-size` and `--rss-limit` (or `PAGE_SIZE` and `RSS_LIMIT`). The profile About text is capped at 6,000 characters, changeable with `--profile-about-max-length`. Running locally shows paging with seeded data.

The code is on [GitHub](https://github.com/can3p/pcom).
