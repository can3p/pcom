# Product rules

How pcom behaves on purpose, with the reason where it isn't obvious. The [user guide](guide/overview.md)
describes the same behavior for users; this file is for whoever changes the code, so a deliberate choice isn't
"fixed" by accident. Numbers that are settings (`pkg/config`, listed by `web serve --help`) are named by
their setting, not their default.

A decision about how pcom behaves is written here, under its area, when it is made; the reasoning and the
options weighed go into the history of the wave that built it (`docs/archive/history/`).

## Accounts and login

- **Signups are closed on purpose.** Bots abused the endpoints. The waiting list stays; `FORCE_SIGNUP`
  opens signups, and reopening them with bot protection is #123.
- **No passwords.** Login is a one-time code mailed to the address and typed on the page where the email
  was entered, never a link. A code belongs to the login attempt in that browser's session and works once.
  Its lifetime, tries and mailing rate are the `LOGIN_*` settings. After `LOGIN_WRONG_TRIES` wrong codes on a
  user's attempts within the window, no code logs them in and none is mailed until the window passes.
- **Accounts are created before they are confirmed.** Accepting an invitation or an open signup creates the
  user and mails a code, typed on the same page. An unconfirmed user can also log in from the login page,
  which confirms them. Accounts nobody confirms within `LOGIN_UNCONFIRMED_LIFETIME` are deleted, except
  those an operator gave invitations to.
- **One account per mailbox.** Signup treats two addresses as one mailbox when they deliver to the same place
  (`pgsession.CanonicalEmail`, which uses `emailaddr.Parse`): a `+tag` on every domain; Gmail's extra dots and
  `googlemail.com`; Outlook, Hotmail and Live; iCloud and Proton aliases; Yahoo's `-keyword`; and Fastmail's
  `x@user.fastmail.com`. Existing `users.email_canonical` values are not recomputed, so an alias of an existing
  account on a provider added later can still sign up; signups are closed, which keeps that small. Invitations
  may still go to a `+tag` address. A second pending invitation to the same address, compared lowercased, is a
  form error, backed by a partial unique index.
- **Signup refuses disposable addresses and accepts relays.** Relays (duck.com, SimpleLogin, Apple Hide My Email)
  are usually a real person hiding their address, so they are a mailbox like any other. Signup checks no DNS:
  it is closed, and the check belongs with reopening it (#123).
- **The operator's way in when mail is down** is `web admin login-code --email`, which prints a fresh code
  for the user's newest open attempt. It must run with the server's `LOGIN_*` settings.
- **`users.pwdhash` is unused.** Migration M4 on the branch `feat/f5-drop-pwdhash` drops it; it merges, as
  its own PR, once login codes have run in production, because fly's `release_command` migrates on every
  deploy.

## Visibility and feeds

- **A public post is public by URL.** `/posts/:id` shows a public post to everybody, whatever the author's
  profile visibility. Lists don't: the public index and `/rss/public` only include authors whose profile
  visibility allows it.
- **Anonymous visitors land on the public index** at `/`; `/explore` is for logged-in users, and an anonymous
  visitor is sent to `/`. The anonymous menu is Home, Sign up and Login; `/articles/:id` serves the legal
  pages.
- **Exports follow visibility.** Anyone who may see a post may export it as a zip.
- **Lists page by cursor**, `PAGE_SIZE` items at a time behind a "Load more" button: the feed, explore, the
  public index and a journal. Every RSS output holds the latest `RSS_LIMIT` items, dated by publication,
  not creation.
- **Deleting a post that was never saved stores nothing.**

## Comments

- **Only the author edits a comment**, and only while they may still comment on the post. An edit that
  changes the body notifies the same people as a new comment, with "edited" variants of the comment mails.
  No history is kept; the marker shows the last edit time.
- **Comment mails and feed entries reach only who may read the comments now.** A participant who lost the
  connection gets no more of them, and nobody hears about the comments of a post that is a draft again. The
  asker of a prompt is mailed the answer only if they may read it.
- **A mail shows what is true when it goes out.** Mail is built when it is sent, not when the change that
  causes it is saved: a post edited before its notifications go out is mailed as edited, a reader who may no
  longer see the post by then gets nothing, and a post made a draft again (or deleted) before then is mailed
  to nobody. Two edits of a comment before its mails go out send each recipient one mail, showing the last.
- **Nobody comments on a draft**, its author included, also on a post that was published before; the
  comment form and the Reply and Edit actions are hidden there.

## Email subscriptions

A reader without an account subscribes by email to an author's public posts (`pkg/service/subscriptions`).
Subscribers are a third audience of mail, after accounts and addresses that have none (`docs/guide/subscriptions.md`
is the user's view).

- **Only visitors who aren't logged in see the subscribe form**: behind a "Subscribe by email" button under the
  journal's title (closed until pressed), and after the article of public posts, of an author who takes subscriptions.
- **The author opts in, and can be paused.** `users.email_subscriptions_enabled` is off until the author turns it on
  under Audience in Settings. Readers can subscribe only while the journal is open to everybody on the internet and
  the setting is on. Otherwise the author's subscriptions are paused: they are kept, nothing is mailed, the subscribe
  form is hidden, and the manage page says that the author isn't sending posts by email. Narrowing the journal turns
  the setting off (its checkbox is disabled, so the form sends none) and keeps the subscriptions.
- **Settings warns before the author cuts subscribers off.** Switching the setting off or narrowing the journal
  while there are subscribers shows "N people subscribed by email won't get your posts while …". Subscribers are
  told nothing about it.
- **A paused author's subscribers can still confirm.** The confirmation page works, the author is told about the new
  subscriber, and nothing is mailed to subscribers until the author is open again.
- **The author sees the addresses.** Confirmed subscribers are listed on the controls page with the date each
  subscribed, and the author can remove one. A removed subscriber gets no mail, and the author gets no mail for a
  removal. Pending subscriptions are neither counted nor listed. The subscribe form, the confirmation page and the
  privacy policy say that the author sees the address, which is why the reader is told.
- **The author is mailed** when a subscription is confirmed, not when it is requested, so that bots cannot fill the
  author's inbox, and when a subscriber unsubscribes. Both mails name the address.
- **Which posts are mailed.** Only public posts, and each at most once: `posts.subscribers_mailed_at` records it, so
  republishing after a draft, or switching visibility back and forth, mails nobody twice. The post form offers one
  checkbox, "Email this post to N subscribers", when the post is public, the author can be subscribed to, there is at
  least one subscriber, and the post was never mailed to them. It is checked on a first publish, and unchecked when an
  already published post becomes public. An explicit request mails a published public post that was never mailed.
  An autosave never mails. Mails to connections are unchanged, and a subscriber who is also a connection gets both.
- **Confirming is a button, not a link.** Opening a confirmation link changes nothing, because mail scanners open
  links; the page has a Confirm button. Confirming an active subscription again changes nothing. An unknown or expired
  link says "This link has expired" and sends the reader to subscribe again from the author's journal. Unsubscribing
  deletes the subscription, and the subscriber's row once it has no subscriptions left. The manage page lists the
  authors the address follows, marks the paused ones, and ends one or all of them with a POST that asks first.
- **The Unsubscribe link in a mail opens the manage page** (a GET that changes nothing); the reader unsubscribes there
  with a POST. The one-click unsubscribe that mail clients post (`List-Unsubscribe-Post`, RFC 8058) is the one form
  exempt from CSRF, since the token in its URL is what authorizes it. Post mails to subscribers carry those headers because
  Gmail and Yahoo require them of bulk senders.
- **Lost the link.** The page for it mails the manage link to the address when the address has subscriptions. It gives
  the same answer whether or not it does, so it shows nothing about who subscribes.
- **Caps are by the target address**, which an attacker cannot rotate: one open confirmation per address and author,
  and a number of confirmation and manage-link mails per address per day (`SUBSCRIBE_CONFIRMS_PER_ADDRESS`,
  `SUBSCRIBE_MANAGE_LINKS_PER_ADDRESS`). Past the cap the answer is the same and nothing is sent. The whole site has an
  hourly cap on confirmation mails (`SUBSCRIBE_CONFIRMS_PER_HOUR`); past it, the subscribe and manage-link forms say
  that subscriptions are paused, and the admin gets one mail an hour. Only confirmation mails count toward it, so
  refused addresses can't pause subscriptions for everybody.
- **A reader pays for each attempt in the browser.** Subscribing needs JavaScript, which solves a proof of work: the
  leading zero bits of a SHA-256 hash, `SUBSCRIBE_POW_DIFFICULTY` (15 bits by default), about a second for a person and
  a cost to a bot on every attempt. A challenge is signed, expires (`SUBSCRIBE_POW_MAX_AGE`) and works once. A
  honeypot field that people never see, and a minimum time between showing the form and submitting it
  (`SUBSCRIBE_MIN_FILL_TIME`), turn away bots; both answer as if the subscription succeeded.
- **Addresses are checked for mail, not only for syntax** (`pkg/emailaddr`). A disposable address is refused, with
  "Please use a permanent address", and logged by domain only; there is no admin mail, since bots would flood it.
  Relays are accepted: their users are the readers this suits, and the author sees the relay address. An address whose
  domain can't receive mail is refused ("This address can't receive mail") when its domain doesn't exist, has a null
  MX, or has neither MX nor address records. A DNS error or timeout lets the address through, so a slow resolver blocks
  nobody. The DNS check runs after the proof of work and the caps, so a bot can't make pcom resolve names for free, and
  outside any transaction, so a slow resolver holds no lock. A domain the check accepted is remembered for a while
  (`EMAIL_MX_CACHE`), so asking again costs no lookup; a refused domain is not, so it is checked again next time and a
  domain fixed or registered since is accepted, since a problem on pcom's side, a stale answer included, never refuses
  an address. Concurrent checks of one domain share one lookup. The site makes at most
  `SUBSCRIBE_DOMAIN_LOOKUPS_PER_HOUR` lookups an hour; past that the check is skipped and the address goes through, as
  on a timeout, and the skip is logged with the domain only.
- **Pending subscriptions expire** after `SUBSCRIBE_CONFIRM_LIFETIME`, and a pruner deletes them with the subscribers
  left without subscriptions and the mail log rows no cap counts any more (`SUBSCRIBE_PRUNE_EVERY`).
- **Confirmed subscriptions last until** the subscriber unsubscribes, the author removes them, or the author's account
  is deleted.

## Profiles

- **The About text is shown on the journal only**, under an "About" heading, capped by
  `PROFILE_ABOUT_MAX_LENGTH`.
- **User CSS targets the `us-*` classes, and they are a contract.** A user's styles load on their journal,
  their posts, the feed and explore, inside `.user-styles-applied`. The `us-*` classes keep their names and the
  elements they mark (the list is under Styles in `docs/guide/settings.md`), so a redesign doesn't break
  people's styles; every other class may change. `TestTemplatesKeepUserStyleHooks` fails when one disappears.
- **The username in the header opens your journal**; Settings has its own link in the menu, next to Controls.

## Grammar check and style review

The post editor has two buttons, **Check grammar** and **Style review**, which send the post's subject and body
to Claude through the server. Users see only the words "grammar check" and "style review"; "proofreading" is the
code's name for the feature (`pkg/proofread`, `PROOFREAD_*`).

- **Who and when.** Every logged-in user, in the post editor only (not comments, not the profile text). A call
  runs only when its button is pressed; nothing runs as the user types. The feature is off unless
  `PROOFREAD_ANTHROPIC_KEY` is set; its settings and their defaults are in `docs/running.md`.
- **The key stays on the server.** The browser talks only to pcom, never to Anthropic.
- **Limits are per user and per kind.** Grammar checks and style reviews have separate hourly limits, counted
  from the user's requests of that kind with `status = 'ok'`, so a failed call is not counted. An optional
  site-wide daily cap can close both. Each request is a row in `proofread_requests`; the numbers are the
  `PROOFREAD_*` settings.
- **Nothing changes the text without a click.** A suggestion from a grammar check is applied or dismissed by
  the author. A style note is never a fix: it is marked done or kept as is, and the text stays as the author
  wrote it.
- **The subject is part of both kinds.** A grammar fix may change the subject; a style note may be about the
  title, and clicking it highlights the subject field.
- **Grammar results live on the page.** Only the author's dismissals are kept, per post, in the browser's
  `localStorage`, so a later check in that browser doesn't show them again. Another browser shows them again.
  A new draft's dismissals move to the post when it is first saved.
- **Style reviews are stored.** Each review is kept with its post: the text reviewed, its notes, their status and
  the author's reasons. The next review is told what was said before, what the author did and why ("Keep as
  is" takes an optional reason for that). Deleting a post deletes its reviews; "Start fresh" deletes the post's
  reviews without deleting the post. The post's request rows stay, without the post, until the account is
  deleted.
- **Grammar-check text is not stored.** pcom keeps only the request row for a grammar check: user, post, kind,
  model, token counts, ok or failed, and time. The privacy policy (`cmd/web/client/articles/privacy_policy.md`) says the same.
- **A disclosure is always visible** wherever the buttons are: the line under the limits says that Claude, an
  external AI model from Anthropic, receives the subject and text only when a button is pressed. It is not behind
  a tooltip, because the author must see it before pressing.
- **A new post has no review until it is saved.** Style review is disabled until the first autosave gives the
  post an id.

## Notes

Notes are private snippets, typed or dictated, that the author later uses in posts. The user guide's
[notes page](guide/notes.md) describes the screens; the rules are here.

- **Private, always.** A note has no visibility, feed, RSS, comments or mail. Every read and write checks that
  the note is the actor's; a note of somebody else is `ErrNotFound`, never forbidden. Recordings are served only
  by `GET /controls/notes/:id/audio`, and never by `user-media` or the CDN, which serve public files by name. A
  recording is a row of `note_recordings`, not a `media_uploads` image, so a note's recording can't be an image.
- **Tags are columns, the text is the note.** `tags` (set by actions) and `text_tags` (the `#word`s of the body,
  rewritten on every save) are both filtered on as `tags || text_tags`. Editing the text edits the text tags, and
  a tag from an action goes with its chip.
- **One search box.** The list narrows by one query (`notes.ParseSearch`): `#tag` must be on the
  note, `-#tag` must not, and every other word is text searched for in the body, case-insensitively. A tag token
  is normalized like any tag, and one that can't be a tag (`#`, `-#`, `#2024`) is text. A tag both included and
  excluded is excluded. Nothing is hidden by a preference: a tag the author no longer wants to see is left out by
  `-#tag`, so what the list shows is always what its query says. The whole search is the `q` parameter; the
  source is the one other (`source`). The page shows the query's parts as chips, each linking to the query
  without it, and every link that adds a tag (a tag on a note, the author's tags under the box) adds it to the
  query on show.
- **Every tag shows.** A note on the list, and a card in the post editor's Notes panel, shows all its tags, the
  ones set by hand and the `#word`s of its text, so a tag is found by seeing it. The panel also offers the
  author's most used tags under its search box (`NOTE_PANEL_TAGS` of them); a tap on any tag adds `#tag` to the
  panel's search.
- **The list page captures too.** Its box saves typed text with the tags typed next to it (separated by spaces or
  commas, `#` optional; the recent tags are the tags of the newest notes) and the markdown toolbar's photos, which
  go into the text as images. A recording picked from the device becomes a note through the same upload as the
  capture page's (`POST /controls/action/note_audio`), and a note without a recording can get one on its edit page.
- **A source is a title.** It has no author, link, kind or page: a title names a book, an article or a talk well
  enough, and every extra field was one more thing to fill in on a phone. Its match key is the title lowercased with
  its spacing collapsed (`model.NoteSourceMatchKey`), unique per author, so the same title typed twice is one
  source. A note's forms choose its source from the author's own (a `<select>` saved with the note, never free
  text, so autosave can't make a source from a half-typed title); a source id of somebody else is not found.
  Sources are added and renamed on the Tags and sources screen. Renaming a source to the title of another merges
  them, as renaming a tag does: its notes move there and it goes, so there is no separate merge. The capture API
  and the bulk Source action take a title and find or make the source.
- **Deleting is final.** No trash, as for posts. The note, its photo links and its recording go; the recording's
  object and row go when the `notes.delete_audio` job runs. Photo files stay: an image is public by
  URL, like a post's, and image deletion is #278. Transcription counts (`stt_requests`) stay with `note_id` null,
  so deleting and recording again does not reset the daily minutes. Text already inserted into a post stays there.
- **A recording is transcribed once.** The upload enqueues `notes.transcribe` in its own transaction, unique per
  note and recording. The job re-checks the note and the recording when it runs, and does nothing if either is gone. The
  transcript becomes the body; editing it later never calls the provider. Language is detected, never set in
  advance: a forced language makes speech in any other language come out translated or garbled, so a per-user
  setting would spoil every memo in another language. "Transcribe again as…" sends the recording once more with the
  language forced, for a memo the detection got wrong; it counts against the limits and replaces the text, after a
  confirmation whenever the note has text (the note doesn't keep its last transcript to compare with).
- **Limits.** `STT_MINUTES_PER_DAY` per user and `STT_SITE_MINUTES_PER_DAY` for the site are counted over the last
  24 hours from `stt_requests` rows with `status = 'ok'`, as the proofreading limits are. Without a provider
  (`STT_PROVIDER` empty) there is no record button, and typed notes work.
- **Quotes never lose text.** Shared text is stored as a blockquote, a lone link stays a link, and typed text is
  stored as typed. A quote takes the previous quote's source only within `NOTE_QUOTE_SOURCE_WINDOW`; otherwise it
  has none until the author gives it one. Its attribution line in a post is `— *Title*`.
- **The list is flat.** Notes are listed newest capture first, never grouped by when they were captured. There
  are no tag suggestions: the recent tags by the capture box and the author's tags under the search are the way
  to a tag. A note inserted into a post is still recorded (`note_post_links`).
- **Photos are ordinary public images.** A note's photo is a `media_uploads` image served by `user-media` and the
  CDN, public to whoever has its random URL, like a draft's images, so it goes into a post as it is. Only
  PNG, JPEG and WebP are accepted, and a HEIC photo (an iPhone's) is converted to JPEG on upload with its
  orientation applied and its metadata, GPS included, dropped; the size cap applies to the upload as sent. The
  image needs libvips with a HEIF loader (`vips-heif` on Alpine); without it HEIC is refused with a message. Text in a photo is recognized on the phone (Live Text,
  Lens), never on the server. Quotes come in one at a time by sharing or copying; there is no Kindle notebook
  import.
- **Speech-to-text is pluggable and cheap.** `STT_PROVIDER` picks a backend in `pkg/stt`; the first one speaks
  the OpenAI-compatible `/audio/transcriptions` API and defaults to Groq's `whisper-large-v3-turbo`, the cheapest
  multilingual option weighed. Recordings are sent as recorded (Opus in WebM or AAC in MP4): the server never
  transcodes.
- **No location, no AI.** No location is stored with a note. No model call is made on notes: no transcript
  cleanup, no tags by a model.
- **Capture is stop-is-save.** Stopping a recording or taking a photo creates the note, with nothing to fill in.
  The capture is written to the phone's IndexedDB before it is uploaded, and the service worker
  (`/controls/notes/sw.js`, scoped to `/controls/notes/`) uploads it with retries. `captured_at` is the time of the
  capture, not of the upload.
- **The share target is CSRF-exempt**, because a share sheet can't send a token. A cross-site POST carries no
  session cookie (SameSite=Lax), so a forged share lands on the login page. The only thing a share can do is add a
  note to the sharing user's own notes. A share reads only its title, text, url and files: it can't set tags, a
  source or `captured_at`. The capture page's service worker sends a share at once and queues it on the phone only when the
  network fails; the queued share uploads when the network returns.
- **API keys are scoped.** A `notes` key opens `POST` and `GET /api/v1/notes` and gets `403` on every other route;
  a `full` key opens the whole API. Keys are stored as a SHA-256 hash and shown once, at creation; they are named, show their last use, and
  revoking one is immediate. A missing or bad key gets `401`.
  The notes API has no edit or delete: a phone can't change what it has stored.

## Interface

- **Every form saves in place** and reports next to its submit button (saved / not saved), with field errors
  under the field: no page reload after a form, and no toast for a form's result. Toasts are only for the
  results of actions without a form. Destructive actions are red and always ask first.
- **A first visit stays light**: CSS, fonts and JS have byte budgets that `TestFirstVisitWeight` enforces,
  because the site must open fast on a phone. pcom uses no CSS framework.
- **Mails share one layout** in the site's light colours (its dark ones where the mail client follows dark mode), with system fonts and no images of their own or tracking. The HTML footer says why the reader gets the mail, for each of three audiences: an account at pcom; an address with no account, for invitations, waiting-list confirmations and the subscription confirmation and manage-link mails, where somebody entered the address; and a subscriber, who subscribed to the author's public posts, with links to manage and to unsubscribe, on post mails, which alone carry the one-click unsubscribe headers. The text footer says the same for subscribers.

## Website

- **The project website is the user guide.** It publishes `docs/guide/` and a landing page built from
  them and the README's quick start; developer docs stay in the repository and are linked on GitHub.
