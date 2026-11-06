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
  exempt from CSRF, since the token in its URL is what authorizes it. Subscriber mail carries those headers because
  Gmail and Yahoo require them of bulk senders.
- **Lost the link.** The page for it mails the manage link to the address when the address has subscriptions. It gives
  the same answer whether or not it does, so it shows nothing about who subscribes.
- **Caps are by the target address**, which an attacker cannot rotate: one open confirmation per address and author,
  and a number of confirmation and manage-link mails per address per day (`SUBSCRIBE_CONFIRMS_PER_ADDRESS`,
  `SUBSCRIBE_MANAGE_LINKS_PER_ADDRESS`). Past the cap the answer is the same and nothing is sent. The whole site has an
  hourly cap on confirmation mails (`SUBSCRIBE_CONFIRMS_PER_HOUR`); past it, the subscribe and manage-link forms say
  that subscriptions are paused, and the admin gets one mail an hour.
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
  nobody. The DNS check runs after the proof of work and the caps, so a bot can't make pcom resolve names for free.
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

## Interface

- **Every form saves in place** and reports next to its submit button (saved / not saved), with field errors
  under the field: no page reload after a form, and no toast for a form's result. Toasts are only for the
  results of actions without a form. Destructive actions are red and always ask first.
- **A first visit stays light**: CSS, fonts and JS have byte budgets that `TestFirstVisitWeight` enforces,
  because the site must open fast on a phone. pcom uses no CSS framework.
- **Mails share one layout** in the site's light colours (its dark ones where the mail client follows dark mode), with system fonts and no images of their own or tracking. The HTML footer says why the reader gets the mail, for each of three audiences: an account at pcom; an address with no account, for invitations and waiting-list confirmations, where somebody entered the address; and a subscriber, who subscribed to the author's public posts, with links to manage and to unsubscribe. The text footer says the same for subscribers.

## Website

- **The project website is the user guide.** It publishes `docs/guide/` and a landing page built from
  them and the README's quick start; developer docs stay in the repository and are linked on GitHub.
