-- River's migration 001 (create river migration), from github.com/riverqueue/river v0.51.0,
-- exported with rivermigrate.AllVersions. Each River version is its own file,
-- as River's migrator runs each in its own transaction; the version is one
-- statement block, since sql-migrate would split its functions at their
-- semicolons. See pkg/jobs.
-- +migrate Up
-- +migrate StatementBegin
CREATE TABLE river_migration(
  id bigserial PRIMARY KEY,
  created_at timestamptz NOT NULL DEFAULT NOW(),
  version bigint NOT NULL,
  CONSTRAINT version CHECK (version >= 1)
);

CREATE UNIQUE INDEX ON river_migration USING btree(version);
-- +migrate StatementEnd

-- +migrate Down
-- +migrate StatementBegin
DROP TABLE river_migration;
-- +migrate StatementEnd
