# WB — Bug-fix wave (2026-09-29, branch `fix/wb-survey-bugs`)

Fixed 31 issues in 12 tasks over three rounds, one commit per task: #108–#117,
#119–#122, #139–#143, #147, #148, #151, #152, #154, #156–#160, #163, #167 and
#168. The wave took the filed bugs #139–#143, #147 and #148 as well, and
the owner decided Q7 (the zip export follows post visibility) and #168
(a second pending invitation to an address is rejected). Rounds existed
because `cmd/web/main.go` held the routes, the RSS and zip handlers and
several `log.Fatal`s, so one task per round owned it; JS/SCSS tasks never
shared a round, because assets are built once per round.

What changed for users and operators:

- Emails are stored lowercased and trimmed in `users`, `user_invitations`
  and `user_signup_requests`, and CHECK constraints enforce it (the owner
  checked the production tables by hand first). Login is one exact lookup
  on the normalized input; a legacy sha256 hash is replaced by argon2id on
  login. Login and logout rotate the session.
- The private RSS feed has its own read-only token (`user_feed_tokens`),
  regenerated from settings; old feed URLs carrying an API key are plain
  404s, as the owner didn't want to keep compatibility. `pkg/repo` exists now, holding the feed token queries.
- Invitation mails are keyed by the invitation, a partial unique index
  allows one pending invitation per address, and the waiting list, signup
  and invitation checks compare addresses case-insensitively.
- Request handling no longer calls `log.Fatal`; HTML emails escape user
  content. Every route with a UUID path param, the API's included, goes
  through the `requireUUIDParam` middleware.

What turned out wrong along the way:

- **B's first design locked users out.** Lowercasing `users.email` in a
  migration destroyed the spelling a legacy hash was salted with, so any
  invited user with a capital letter could never log in, and accounts that
  differed only in case were skipped. It was first reworked to look up
  case-insensitively; once the owner had checked the production tables, it
  became a plain normalization with database constraints instead.
- **B broke change password.** Login rehashed to argon2id while the change
  password form still compared sha256 hashes. Caught from B's own report,
  not by a test; the form now uses the same check.
- **#108's first fix sorted by a UUID.** `ORDER BY id DESC` on UUID ids is
  random, and the test inserted rows in the order it expected, so it passed
  either way. The coordinator's own correction first inverted the insertion
  order the wrong way; on a +02:00 host the test also couldn't fail because
  of #171. The comments query's order can't be observed through
  `SinglePost`, which re-sorts, so it has no test.
- **#141's first test couldn't fail.** The race was made deterministic by
  clicking Delete inside `htmx:afterSwap`.
- **`make cover-check` passed on stale data.** A fresh `make cover` showed
  `pkg/auth` below its floor (89.0% against 91.6%), because the auth rework
  added error branches. Tests for the reachable ones brought it to 91.1%;
  the rest are session-save, session-rotation and database-error paths
  that need a fault injector, so the owner lowered the floor to 90.1%, the
  one exception so far to "floors only ever rise".

Left out: #171 (timestamps without a time zone; filed during WB, not
scheduled), a test for #115's database-error 500, a test for #111 (R2
replaces the flags), and the session-save error branches in `pkg/auth`.
Still for later, after R1: #123 (signups with bot protection) and #124
(feed pagination, which needs W6.B5's feed browser tests).

**Cost.** 1 coordinator session (144 turns, peak 255k context, 146k of tool
results; flags: `cat`×9) and 17 subagents: 13 task runs (one a rework at a
higher tier) and 4 assertion audits, about 600 turns together, medians
29 turns and 63k peak. The busiest subagents were B (strong: 109 turns,
87k peak) and D (60 turns, 125k peak). Across all agents, tool results fed
710k through Bash and 277k through Read; wasteful calls: `cat`×31,
verbose×1, full-read×1, raw-build×1. Skills: wave-run and wave-close once,
frontend-htmx by 2 subagents; model-shape and test-failure were never used.
The coordinator peak is close to W4's (276k) and far above W5's one-task
70k: most of it came from reviewing diffs of production code, which a
bug-fix wave needs and test waves didn't, plus about ten minutes of
safety-classifier outage retries.
