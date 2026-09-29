## WB — Bug-fix wave

After the safety net is in place, and **before** any refactor, so that R1–R5
stay pure refactors. Each fix removes the matching `t.Skip`, and that test is
the proof. Prompts come from `task_prompt.py wb <task>`, which pastes the
row and each issue's text.

Tasks are grouped by the files they edit. `cmd/web/main.go` holds the routes,
the RSS and zip handlers and several `log.Fatal`s, so one task per round owns
it. Tasks that change `cmd/web/client/js` or `scss` never share a round,
because assets are built once per round by the coordinator. Only D adds a
column and regenerates models; B's and B2's migrations change data and
indexes only.

| Task | Round | Issues | Tier | Owns | Notes |
|---|---|---|---|---|---|
| A | 1 | #109, #111, #139, #151, #152, #154 | mid | `cmd/web/main.go`, `cmd/web/actions.go`, `cmd/web/client/html/` for #139, their skipped tests in `e2e/` and `e2e/browser` | #111 is superseded by R2 but cheap now; #152: check the other UUID path params too |
| B | 1 | #114 + #119, #122, #160 | strong | `pkg/auth`, `pkg/pgsession`, `pkg/forms/form_login.go`, `pkg/mail/valid.go`, one migration, their tests | lowercase emails in a migration; verify legacy hashes against both the original and the lowercased email, then rehash to argon2id on login; rotate the session on login; `Login` returns DB errors |
| G | 1 | #108 | cheap | `pkg/postops/post_prompts.go`, `pkg/web/func.go`, the other `ORDER BY ?` call sites, their tests | 4 call sites |
| H | 1 | #116 | cheap | `pkg/feedops/feeder/feeder.go`, its tests | |
| I | 1 | #156 | cheap | `pkg/web/api.go`, `pkg/web/api_test.go` | the skipped test forces a non-UTC zone |
| J | 1 | #141, #142, #143, #157, #163 | mid | `pkg/forms/form_post_new.go`, `pkg/forms/validation/fields.go`, `cmd/web/client/html/form--post.html`, the post editor's Stimulus controller, their tests in `pkg/forms` and `e2e/browser/writing_test.go` | browser tests; #163 per Q16: delete on a never-saved post stores nothing |
| K | 1 | #147, #148, #159 | cheap | `pkg/media/upload.go`, `pkg/forms/validation/email.go`, `pkg/forms/form_user_styles.go`, `pkg/forms/form_signup.go`, their tests | |
| B2 | 2 (after B) | #158, #167, #168 | mid | `pkg/forms/form_signup_waiting_list.go`, `pkg/forms/form_send_invite.go`, `pkg/mail/invite.go`, one migration, their tests | #167: key the mail by `invite.ID`; #168: reject a second pending invite for the same address, with a partial unique index on the lowercased pending address |
| D | 2 (after A) | #110, #115, #121 | mid | `cmd/web/main.go`, `pkg/postops/export.go`, `pkg/links`, the settings page showing the private RSS URL, one migration, regenerated models, their tests | #110 per Q7: anyone who can see the post may export it; #121: a separate read-only feed token |
| F | 2 (after A) | #113, #117 | mid | `pkg/mail/sender/dbsender`, `pkg/userops/connections.go`, `cmd/web/actions.go` call sites, their tests | locks inside the transaction; a decided request can't be decided again |
| L | 2 (after J) | #140 | cheap | the navbar template and its JS/SCSS, `e2e/browser/navigation_test.go` | CSP style-src-attr on the mobile menu |
| C | 3 (after B2, D) | #112, #120 | mid | `pkg/mail`, `pkg/admin`, `pkg/markdown/modify.go`, the `log.Fatal`s in `cmd/web/main.go`, their tests | return errors; #120: `html.EscapeString` now, R4 makes it structural |

**Future, not part of WB:** #123 (re-enable signups with bot protection;
signups are off on purpose) and #124 (feed and explore pagination). Both come
after R1, because they touch routes R1 moves. #124 needs W6.B5's feed browser tests
in place first.

### Known bugs (for de-duplication)

Filed: #108–#117, #119–#124, #139–#143, #147, #148, #151, #152, #154, #156–#160, #163, #167 and #168. Already fixed:
the foreign-post delete through `DELETE /api/v1/posts/:id`, in PR #118
(merged); W2.D3 and W3.E4 test the ownership check as a normal, non-skipped
test. Drafts served to non-authors at `/posts/:id` (found by W3.E1) are fixed
in PR #153, stacked on W3. W2.D2a found that the journal (`UserHome`) loaded
direct-only and second-degree posts for a logged-in visitor unrelated to the
author (the template never rendered them); fixed in its own PR, stacked on
W2.
