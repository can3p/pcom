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

## Profiles

- **The About text is shown on the journal only**, under an "About" heading, capped by
  `PROFILE_ABOUT_MAX_LENGTH`.

## Website

- **The project website is the user guide.** It publishes `docs/guide/` and a landing page built from
  them and the README's quick start; developer docs stay in the repository and are linked on GitHub.
