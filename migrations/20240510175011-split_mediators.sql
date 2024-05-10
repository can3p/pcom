
-- +migrate Up
create table user_connection_mediators (
  id uuid primary key,
  mediation_id uuid not null references user_connection_mediation_requests(id),
  user_id uuid not null references users(id),
  decision connection_mediation_decision not null,
  decided_at timestamp not null,
  mediator_note varchar
);

create unique index on user_connection_mediators(mediation_id, user_id);

alter table user_connection_mediation_requests
  drop column mediator_user_id,
  drop column mediator_decision,
  drop column mediator_decided_at,
  drop column mediator_note;

-- +migrate Down
