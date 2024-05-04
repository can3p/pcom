
-- +migrate Up
create table user_connections (
  id uuid primary key,
  -- we thing we have an undirected graph there
  -- if a connection between users is established, it's mutual
  user1_id uuid references users(id) not null,
  user2_id uuid references users(id) not null,
  created_at timestamp not null,
  updated_at timestamp not null
);

create unique index on user_connections(user1_id, user2_id);

-- +migrate Down
drop index user_connections_user1_id_user2_id_idx;
drop table user_connections;
