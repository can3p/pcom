-- +migrate Up

-- Where a note comes from: a book or a web page. match_key is the title and
-- author normalized by the notes service, so the same book typed twice is one
-- source.
create table note_sources (
  id uuid primary key,
  user_id uuid not null references users(id) on delete cascade,
  kind text not null check (kind in ('book', 'web')),
  title text not null,
  author text,
  url text,
  match_key text not null,
  unique (user_id, match_key)
);

-- What media_uploads holds: an image (public by URL, note photos included) or
-- a note's recording, which only its owner may fetch.
create type media_kind as enum ('image', 'note_audio');

alter table media_uploads add column kind media_kind not null default 'image';

create type note_transcription_status as enum ('none', 'pending', 'done', 'failed');

-- A private note. tags are the ones set by hand (a chip, bulk tagging, the
-- focus, the API); text_tags are the #words of the body, derived by the
-- service on every write. A recording's transcription becomes the body.
create table notes (
  id uuid primary key,
  user_id uuid not null references users(id) on delete cascade,
  body text not null default '',
  tags text[] not null default '{}',
  source_id uuid references note_sources(id) on delete set null,
  locator text,
  audio_upload_id uuid references media_uploads(id) on delete set null,
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

-- The tag captures get until it is changed or NOTE_FOCUS_IDLE passes after
-- notes_focus_at, the last capture under it.
alter table users add column notes_focus_tag text;
alter table users add column notes_focus_at timestamptz;

-- +migrate Down

alter table users drop column notes_focus_at;
alter table users drop column notes_focus_tag;
drop table stt_requests;
drop table note_post_links;
drop type note_post_link_how;
drop table note_images;
drop table notes;
drop type note_transcription_status;
alter table media_uploads drop column kind;
drop type media_kind;
drop table note_sources;
