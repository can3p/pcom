-- +migrate Up

-- Where a note comes from, named by its title alone. match_key is the title
-- normalized by model.NoteSourceMatchKey, so the same title typed twice is one
-- source.
create table note_sources (
  id uuid primary key,
  user_id uuid not null references users(id) on delete cascade,
  title text not null,
  match_key text not null,
  unique (user_id, match_key)
);

-- A note's recording: the object fname (under audio/) in the media bucket,
-- which only its owner may fetch. It has a table of its own, apart from
-- media_uploads (public images), so a note's recording can never be an image.
create table note_recordings (
  id uuid primary key,
  user_id uuid not null references users(id) on delete cascade,
  fname text not null unique,
  content_type text not null,
  size_bytes bigint not null,
  created_at timestamptz not null default now()
);

create type note_transcription_status as enum ('none', 'pending', 'done', 'failed');

-- A private note. tags are the ones set by hand (a chip, bulk tagging, the
-- API); text_tags are the #words of the body, derived by the
-- service on every write. A recording's transcription becomes the body.
create table notes (
  id uuid primary key,
  user_id uuid not null references users(id) on delete cascade,
  body text not null default '',
  tags text[] not null default '{}',
  source_id uuid references note_sources(id) on delete set null,
  recording_id uuid references note_recordings(id) on delete set null,
  text_tags text[] not null default '{}',
  transcription_status note_transcription_status not null default 'none',
  transcription_error text,
  captured_at timestamptz not null default now(),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

-- The list and its cursor: newest capture first, the id breaking ties.
create index on notes(user_id, captured_at desc, id);
create index on notes(source_id);
create index on notes(recording_id);
-- Filtering by tag queries this exact expression.
create index on notes using gin ((tags || text_tags));

-- The photos of a note, in the order of position, shown as a strip and
-- removable one by one without editing the body.
create table note_images (
  note_id uuid not null references notes(id) on delete cascade,
  upload_id uuid not null references media_uploads(id) on delete cascade,
  position integer not null,
  primary key (note_id, upload_id)
);

create type note_post_link_how as enum ('inserted');

-- A note used in a post. Inserting a note that is already linked keeps the row.
create table note_post_links (
  note_id uuid not null references notes(id) on delete cascade,
  post_id uuid not null references posts(id) on delete cascade,
  how note_post_link_how not null,
  created_at timestamptz not null default now(),
  primary key (note_id, post_id)
);

create index on note_post_links(post_id);

-- One row per call to a speech-to-text backend, ok or failed. The daily
-- limits sum the seconds of ok rows, a user's and the site's.
create table stt_requests (
  id uuid primary key,
  user_id uuid not null references users(id) on delete cascade,
  note_id uuid references notes(id) on delete set null,
  provider text not null,
  model text not null,
  seconds integer not null,
  status text not null check (status in ('ok', 'failed')),
  created_at timestamptz not null default now()
);

create index on stt_requests(user_id, created_at);
create index on stt_requests(created_at);

-- +migrate Down

drop table stt_requests;
drop table note_post_links;
drop type note_post_link_how;
drop table note_images;
drop table notes;
drop type note_transcription_status;
drop table note_recordings;
drop table note_sources;
