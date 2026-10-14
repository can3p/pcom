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
- **One account per mailbox.** Signup treats a `+tag`, and on Gmail extra dots or `googlemail.com`, as the same
  mailbox (`pgsession.CanonicalEmail`). Invitations may still go to a `+tag` address. A second pending
  invitation to the same address, compared lowercased, is a form error, backed by a partial unique index.
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
- **Nobody comments on a draft**, its author included, also on a post that was published before; the
  comment form and the Reply and Edit actions are hidden there.

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

## Interface

- **Every form saves in place** and reports next to its submit button (saved / not saved), with field errors
  under the field: no page reload after a form, and no toast for a form's result. Toasts are only for the
  results of actions without a form. Destructive actions are red and always ask first.
- **A first visit stays light**: CSS, fonts and JS have byte budgets that `TestFirstVisitWeight` enforces,
  because the site must open fast on a phone. pcom uses no CSS framework.
- **Mails share one layout** in the site's light colours (its dark ones where the mail client follows dark mode), with system fonts and no images of their own or tracking. The HTML footer says why the reader gets the mail: an account at pcom, or, for invitations and waiting-list confirmations, that somebody entered the address.

## Website

- **The project website is the user guide.** It publishes `docs/guide/` and a landing page built from
  them and the README's quick start; developer docs stay in the repository and are linked on GitHub.
