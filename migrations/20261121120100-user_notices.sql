-- +migrate Up

-- The one-time notices a user has dismissed ("scratch_pad"; the ids are
-- constants in the accounts service). A notice with no row is still shown.
create table user_notices (
  user_id uuid not null references users(id) on delete cascade,
  notice text not null,
  dismissed_at timestamptz not null,
  primary key (user_id, notice)
);

-- +migrate Down
drop table user_notices;
