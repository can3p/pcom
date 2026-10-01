-- +migrate Up

-- The free-form "About" text shown at the top of a user's journal. No row
-- means no section.
create table user_profiles (
  user_id uuid primary key references users(id) on delete cascade,
  about text not null,
  updated_at timestamp not null
);

-- +migrate Down

drop table user_profiles;
