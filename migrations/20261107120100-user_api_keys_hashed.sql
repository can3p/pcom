-- +migrate Up

-- A user may have several keys, each with a name and a scope: full (the whole
-- API) or notes (creating and reading notes only). The existing keys are full.
drop index user_api_keys_user_id_idx;
create index on user_api_keys(user_id);

alter table user_api_keys add column name text not null default '';
alter table user_api_keys add column scope text not null default 'full' check (scope in ('full', 'notes'));
alter table user_api_keys alter column scope drop default;
alter table user_api_keys add column last_used_at timestamptz;

-- A key is stored as the hex SHA-256 of the key as presented
-- (model.HashAPIKey; a key is random, so a fast hash is enough). The existing
-- keys are uuids, presented in their lowercase text form; this is a frozen copy
-- of model.HashAPIKey for them.
alter table user_api_keys add column key_hash text;
update user_api_keys set key_hash = encode(sha256(convert_to(api_key::text, 'UTF8')), 'hex');
alter table user_api_keys alter column key_hash set not null;
alter table user_api_keys drop column api_key;
create unique index on user_api_keys(key_hash);

-- +migrate Down

-- The plain keys are gone, so going down deletes every key.
delete from user_api_keys;
drop index user_api_keys_key_hash_idx;
alter table user_api_keys drop column key_hash;
alter table user_api_keys add column api_key uuid not null;
alter table user_api_keys drop column last_used_at;
alter table user_api_keys drop column scope;
alter table user_api_keys drop column name;
drop index user_api_keys_user_id_idx;
create unique index on user_api_keys(user_id);
