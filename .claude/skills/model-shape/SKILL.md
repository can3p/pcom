---
name: model-shape
description: Look up the fields, relationships and query helpers of a pcom database model (User, Post, PostComment, RSSFeed, ...) without reading the generated pkg/model/core code. Use whenever a task needs a model's shape, writes fixtures or factories, or writes queries.
---

# Model shape without reading generated code

`pkg/model/core` is 1 MB of sqlboiler output; reading it is never the answer (project settings may deny it).

1. `make model` lists every model. `make model T=Post` prints:
   - fields and their Go types (`null.String` means the column is nullable);
   - relationships: the `o.R.<Name>` fields, loaded with `qm.Load("<Name>")`;
   - query helpers: `core.Posts(mods...)`, `core.PostWhere.<Field>.EQ(v)`, `core.PostColumns.<Field>`;
   - finders and methods, without the panicking `...P` variants.
2. Only when that isn't enough, ask for one symbol: `go doc ./pkg/model/core PostWhere`, or `hover` on a use
   of it with the LSP tool.
3. The database schema itself is in `migrations/*.sql`; `grep -n 'CREATE TABLE posts' -A30 migrations/*.sql`
   shows constraints and defaults the model doesn't.

## In tests

- Create and read back fixtures only through the test factories (`pkg/testutil/factory` from W0 on,
  `pkg/feedops/testutil` before). A test body that calls `core.Posts(...)` directly is a test the bob
  migration (R5) has to rewrite.
- If the factory lacks a helper, **stop and report the exact signature you need** (for example
  `factory.Post(t, db, author, factory.WithVisibility(...))`). Don't write the ORM call in the test; the
  coordinator adds helpers in one place.
