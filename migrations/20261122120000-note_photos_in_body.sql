-- +migrate Up

-- A note's photos are markdown images of its body, as a post's are, so the
-- body is the whole note wherever it shows or goes. Each photo becomes an image
-- paragraph, in the order of position, before the note's text.
update notes n
set body = p.images || case when n.body = '' then '' else E'\n\n' || n.body end
from (
  select ni.note_id, string_agg('![](' || mu.uploaded_fname || ')', E'\n\n' order by ni.position) as images
  from note_images ni
  join media_uploads mu on mu.id = ni.upload_id
  group by ni.note_id
) p
where p.note_id = n.id;

drop table note_images;

-- +migrate Down

-- The photos stay in the bodies: they are the note's text now.
create table note_images (
  note_id uuid not null references notes(id) on delete cascade,
  upload_id uuid not null references media_uploads(id) on delete cascade,
  position integer not null,
  primary key (note_id, upload_id)
);
