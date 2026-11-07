# API

## Keys

Every call sends an API key as `Authorization: Bearer <api-key>`. Keys are made in Settings, under "API
keys", each with a name and an access level, and shown once, when they are made: pcom keeps only a hash.
Settings lists every key with its access and when it was last used, and revokes a key, which stops working
at once.

- A **full** key opens the whole API.
- A **notes** key opens only `POST` and `GET /api/v1/notes`, and gets `403` on every other route. It is
  the key for a phone Shortcut: a key stored on a phone can't edit or delete anything.

A call without a key, with a malformed `Authorization` header, or with an unknown or revoked key gets
`401` with `WWW-Authenticate: Bearer`.

## Fetch posts

```
curl -v -H'Authorization: Bearer 01904236-6811-77fe-8076-f7e80f9a8b99' http://localhost:8080/api/v1/posts | jq .
{
  "data": {
    "posts": [
      {
        "id": "018fa64b-0f9e-7933-b44d-33eae44ccfe1",
        "subject": "",
        "md_body": "# We love headers",
        "visibility": "direct_only",
        "is_published": true,
        "public_url": "http://localhost:8080/posts/018fa64b-0f9e-7933-b44d-33eae44ccfe1"
      },
      ...
    ],
    "cursor": ""
  }
}
```

If cursor is not empty, pass it as `cursor` query parameter with the next call to get next page

## Upload an Image

```
curl -v -H'Authorization: Bearer <api-key>' -XPUT -F 'file=@path/to/image.png' http://localhost:8080/api/v1/image
{"data":{"image_id":"0190478c-5592-74ab-9d1a-5cdab598f2dd.png"}}%
```

## Create new Post

```
curl -v -H'Authorization: Bearer <api-key>' -XPOST -d'{ "subject": "test post", "md_body": "is saved\n\n![trololo](0190478c-5592-74ab-9d1a-5cdab598f2dd.png)", "visibility": "direct_only" }' http://localhost:8080/api/v1/posts
{"data":{"id":"01904796-62f7-7a9a-a7bd-1595ed6d1663","public_url":"http://localhost:8080/posts/01904796-62f7-7a9a-a7bd-1595ed6d1663"}}%
```

## Update a Post

```
curl -v -H'Authorization: Bearer <api-key>' -XPOST -d'{ "subject": "test post1", "md_body": "is **saved**\n\n![trololo](0190478c-5592-74ab-9d1a-5cdab598f2dd.png)", "visibility": "direct_only" }' http://localhost:8080/api/v1/posts/01904796-62f7-7a9a-a7bd-1595ed6d1663
{"data":{"id":"01904796-62f7-7a9a-a7bd-1595ed6d1663","public_url":"http://localhost:8080/posts/01904796-62f7-7a9a-a7bd-1595ed6d1663"}}%
```

## Publish a Post

```
curl -v -H'Authorization: Bearer <api-key>' -XPOST -d'{ "subject": "test post1", "md_body": "is **saved**\n\n![trololo](0190478c-5592-74ab-9d1a-5cdab598f2dd.png)", "visibility": "direct_only", "is_published": true }' http://localhost:8080/api/v1/posts/01904796-62f7-7a9a-a7bd-1595ed6d1663
{"data":{"id":"01904796-62f7-7a9a-a7bd-1595ed6d1663","public_url":"http://localhost:8080/posts/01904796-62f7-7a9a-a7bd-1595ed6d1663"}}%
```

### Email subscribers

A public post can also be mailed to your email subscribers, if you let people subscribe to your journal by
email. The optional `notify_subscribers` field of a create or update decides it:

- left out, the post is mailed on the call that publishes it, and not on any other call, such as one that
  makes a published post public;
- `true` mails a published public post, `false` doesn't.

A post is mailed to your subscribers at most once, so `true` on a post that was mailed already changes
nothing. Mails to your connections don't depend on this field.

```
curl -v -H'Authorization: Bearer <api-key>' -XPOST -d'{ "subject": "test post1", "md_body": "is **saved**", "visibility": "public", "is_published": true, "notify_subscribers": false }' http://localhost:8080/api/v1/posts/01904796-62f7-7a9a-a7bd-1595ed6d1663
```

## Delete a Post

```
curl -v -H'Authorization: Bearer <api-key>' -XDELETE http://localhost:8080/api/v1/posts/01904796-62f7-7a9a-a7bd-1595ed6d1663
{"data":null}
```

## Capture a note

`POST /api/v1/notes` makes one note from a `multipart/form-data` (or url-encoded) form. Every field is
optional, but a capture needs text, a photo or a recording.

| Field | |
|---|---|
| `text` | The note's text. |
| `kind` | How the text arrived: `note` (the default: typed, stored as it is), `share` (shared from another app) or `quote` (a passage of a book). Shared and quoted text is stored as a markdown blockquote; a text that is a single link stays a bare link. |
| `tags` | Tags, repeated or comma-separated (`tags=walk,idea`); `#` and case don't matter. The `#words` of the text are tags too. |
| `captured_at` | When the note was taken, RFC 3339 (`2026-11-07T09:30:00+01:00`); now when left out. |
| `source` | The title of the book the note comes from; a source with that title (whatever the case and spacing) is reused, otherwise made. A `quote` without a source takes the source of your previous quote while that was within the focus idle time. |
| `image` | A photo, JPEG, PNG or WebP; repeat the field for several. HEIC is refused: convert it to JPEG first. |
| `audio` | One recording, WebM, Ogg, MP4 (M4A) or MP3, transcribed into the note's text when the server has a speech-to-text backend. |

The notes focus, when set, is added to the note's tags and renewed. The answer is the note:

```
curl -H'Authorization: Bearer <api-key>' -F 'text=Selected passage' -F kind=quote -F source=Dune -F 'image=@page.jpg' http://localhost:8080/api/v1/notes
{"data":{"id":"019a5e1c-…","body":"> Selected passage","tags":[],"source":"Dune","transcription_status":"none","captured_at":"2026-11-07T08:15:42Z","images":["http://localhost:8080/user-media/019a5e1c-….jpg"]}}
```

`transcription_status` is `pending` while a recording waits for its transcription, `done`, `failed`, or
`none` when there is nothing to transcribe. Refused input gets `400` with the message to show and the
field to fix, also on a server that hides error details:

```
{"errors":["A note can have at most 10 tags."],"field":"tags"}
```

A request larger than `NOTE_CAPTURE_MAX_BYTES` (plus `MAX_BODY_SIZE`) gets `413`; one recording may have
at most `NOTE_AUDIO_MAX_BYTES`.

### The requests of the iPhone Shortcut

The "pcom note" Shortcut ([docs/guide/notes-on-a-phone.md](guide/notes-on-a-phone.md)) sends these, with
"Get Contents of URL", method POST, request body Form; `TestAPINotes_ShortcutRequests` (`e2e/`) replays
them:

| Run | Fields |
|---|---|
| Action button | `kind=note`, `captured_at` (Current Date, ISO 8601), `audio` (Record Audio: `Recording.m4a`, `audio/x-m4a`) |
| Share Sheet, text | `kind=share`, `captured_at`, `text` (Shortcut Input) |
| Share Sheet, a link | `kind=share`, `captured_at`, `text` (the URL) |
| Share Sheet, photos | `kind=share`, `captured_at`, `image` once per photo (`IMG_0001.jpeg`, `image/jpeg`) |
| Quote from photo | `kind=quote`, `captured_at`, `text` (Extract Text from Image), `image` (Take Photo, `image/jpeg`) |

The test builds these requests in Go from the table above. To replace them with what the iPhone really
sends, record each run's raw request:

1. Make the Shortcut's URL point at a request bin instead of pcom: any endpoint that keeps the raw body
   and headers, such as a self-hosted `httpbin`-style catcher or `nc -l 8099 > run.http` on a laptop in
   the same Wi-Fi network (`http://<laptop>:8099/api/v1/notes`). Use a throwaway API key, since the
   request carries it.
2. Run the Shortcut each way: from the Action button, from the share sheet with text, a link and two
   photos, and "Quote from photo".
3. Save each request as it arrived, headers and body byte for byte, with the `Authorization` value
   replaced by `Bearer <key>`, in `e2e/testdata/shortcut/` under the names its README lists. Note the
   iOS version in the README.

Whether "Get Contents of URL" runs with the phone locked is recorded there too.

## List notes

`GET /api/v1/notes` pages your notes, newest capture first, like `GET /api/v1/posts`: `limit` is the page
size, from 1 to 100 (left out, it is 1), and a non-empty `cursor` in the answer is passed as `cursor` for
the next page. Filters: `tag` (repeated: notes with every tag), `not` (repeated: notes with none of
them), `inbox=true` (notes without tags), `unused=true` (notes in no post), `q` (text in the body, any case).

```
curl -H'Authorization: Bearer <api-key>' 'http://localhost:8080/api/v1/notes?limit=20&tag=walk'
{"data":{"notes":[{"id":"019a5e1c-…","body":"…","tags":["walk"],"transcription_status":"done","captured_at":"2026-11-07T08:15:42Z","images":[]}],"cursor":"…"}}
```
