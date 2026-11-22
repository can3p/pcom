-- +migrate Up

-- A note attached to a post one by one ('attached'), or one an attached tag
-- would bring that is kept out of the post ('excluded'). Attaching changes no
-- text, so a note can be inserted into a post and attached to it as well:
-- how joins the primary key.
alter type note_post_link_how add value 'attached';
alter type note_post_link_how add value 'excluded';

alter table note_post_links drop constraint note_post_links_pkey;
alter table note_post_links add primary key (note_id, post_id, how);

-- A tag attached to a post: every note of the post's author carrying it, now
-- or later (tags || text_tags), belongs to the post unless excluded.
create table post_note_tags (
  post_id uuid not null references posts(id) on delete cascade,
  tag text not null,
  created_at timestamptz not null default now(),
  primary key (post_id, tag)
);

-- +migrate Down

drop table post_note_tags;

delete from note_post_links where how <> 'inserted';

alter table note_post_links drop constraint note_post_links_pkey;
alter table note_post_links add primary key (note_id, post_id);

alter type note_post_link_how rename to note_post_link_how_old;
create type note_post_link_how as enum ('inserted');
alter table note_post_links alter column how type note_post_link_how using how::text::note_post_link_how;
drop type note_post_link_how_old;
