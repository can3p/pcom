-- River's migration 003 (river job tags non null), from github.com/riverqueue/river v0.51.0,
-- exported with rivermigrate.AllVersions. Each River version is its own file,
-- as River's migrator runs each in its own transaction; the version is one
-- statement block, since sql-migrate would split its functions at their
-- semicolons. See pkg/jobs.
-- +migrate Up
-- +migrate StatementBegin
ALTER TABLE river_job ALTER COLUMN tags SET DEFAULT '{}';
UPDATE river_job SET tags = '{}' WHERE tags IS NULL;
ALTER TABLE river_job ALTER COLUMN tags SET NOT NULL;
-- +migrate StatementEnd

-- +migrate Down
-- +migrate StatementBegin
ALTER TABLE river_job
    ALTER COLUMN tags DROP NOT NULL,
    ALTER COLUMN tags DROP DEFAULT;
-- +migrate StatementEnd
