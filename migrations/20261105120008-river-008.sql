-- River's migration 008 (job id autoincrement), from github.com/riverqueue/river v0.51.0,
-- exported with rivermigrate.AllVersions. Each River version is its own file,
-- as River's migrator runs each in its own transaction; the version is one
-- statement block, since sql-migrate would split its functions at their
-- semicolons. See pkg/jobs.
-- +migrate Up
-- +migrate StatementBegin
-- No-op. Postgres sequences already prevent automatically generated job IDs
-- from being reused.
SELECT 1;
-- +migrate StatementEnd

-- Record the versions as River's own migrator would, so a later River
-- migration can be applied with rivermigrate or exported the same way.
INSERT INTO river_migration (line, version) VALUES
    ('main', 1),
    ('main', 2),
    ('main', 3),
    ('main', 4),
    ('main', 5),
    ('main', 6),
    ('main', 7),
    ('main', 8);

-- +migrate Down
DELETE FROM river_migration WHERE line = 'main';

-- +migrate StatementBegin
-- No-op. Postgres sequences already prevent automatically generated job IDs
-- from being reused.
SELECT 1;
-- +migrate StatementEnd
