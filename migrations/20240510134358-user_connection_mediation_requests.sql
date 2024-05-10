
-- +migrate Up
create type connection_mediation_decision as ENUM ('signed', 'dismissed');
create type connection_request_decision as ENUM ('approved', 'dismissed');

create table user_connection_mediation_requests (
  id uuid primary key,
  who_user_id uuid references users(id) not null,
  target_user_id uuid references users(id) not null,
  mediator_user_id uuid references users(id) not null,
  mediator_decision connection_mediation_decision,
  mediator_decided_at timestamp,
  mediator_note varchar,
  target_decision connection_request_decision,
  target_decided_at timestamp,
  target_note varchar,
  connection_id uuid references user_connections(id),
  created_at timestamp not null,
  updated_at timestamp not null
);

create unique index on user_connection_mediation_requests(connection_id);
create unique index on user_connection_mediation_requests(who_user_id, target_user_id);

-- +migrate Down
