-- +migrate Up

-- The salt of every proof-of-work challenge a form accepted, so a solved
-- challenge is used once. Rows older than the challenge's maximum age are
-- pruned by used_at.
create table pow_challenges_used (
  salt text primary key,
  used_at timestamp not null
);

create index on pow_challenges_used(used_at);

-- +migrate Down

drop table pow_challenges_used;
