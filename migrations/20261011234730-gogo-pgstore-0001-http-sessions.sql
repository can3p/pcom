-- gogo sessions/pgstore: the table and indexes the session store needs. They are
-- antonlindstrom/pgstore's, with IF NOT EXISTS, so this is a no-op where pgstore
-- already created them. Copy it into your migrations with
-- go run github.com/can3p/gogo/sessions/pgstore/cmd/pgstore-migrations -dir migrations
-- +migrate Up
CREATE TABLE IF NOT EXISTS http_sessions (
    id BIGSERIAL PRIMARY KEY,
    key BYTEA,
    data BYTEA,
    created_on TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    modified_on TIMESTAMPTZ,
    expires_on TIMESTAMPTZ);
CREATE INDEX IF NOT EXISTS http_sessions_expiry_idx ON http_sessions (expires_on);
CREATE INDEX IF NOT EXISTS http_sessions_key_idx ON http_sessions (key);

-- +migrate Down
DROP TABLE IF EXISTS http_sessions;
