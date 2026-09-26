## WB — Bug-fix wave

After the safety net is in place, and **before** any refactor, so that R1–R5
stay pure refactors. There is one task per issue, each mid or cheap, and they
are mostly parallel. #109 and #111 both touch `cmd/web/main.go`, so give them
to one task. #114 and #119 both change the password hash, so they go together
as well. Each fix removes the matching `t.Skip`, and that test is the proof.

| Issue | Summary | Notes |
|---|---|---|
| #108 | `ORDER BY ?` placeholder sorts nothing | 4 call sites |
| #109 | Handlers keep running after redirect | `cmd/web/main.go`, `actions.go` |
| #110 | Post zip export: anon panic, empty for non-authors, double close | decide: author-only? (Q7) |
| #111 | `-html` flag unusable; cleanup deferred before err check | superseded by R2 if R2 comes first |
| #112 | `log.Fatal` in the request path | return errors; touches every mail func |
| #113 | `FOR UPDATE` outside the transaction | dbsender double-send |
| #114 + #119 | Email case-sensitivity + argon2id with rehash-on-login | one task (strong): lowercase emails in a migration, verify legacy hashes against both the original and the lowercased email, then rehash to argon2id on login |
| #115 | RSS endpoints return the wrong status codes | |
| #116 | Feed poller swallows errors | |
| #117 | Requests can be decided twice | |
| #120 | Unescaped user content in HTML emails | quick `html.EscapeString` fix here; R4 makes it structural |
| #121 | Private RSS URL carries the read/write API key | separate read-only feed token; needs a migration |
| #122 | Session not rotated on login | small; with the auth work |

**Future, not part of WB:** #123 (re-enable signups with bot protection;
signups are off on purpose) and #124 (feed and explore pagination). Both come
after R1, because they touch routes R1 moves. #124 needs W6.B5's feed browser tests
in place first.

### Known bugs (for de-duplication)

Filed: #108–#117 and #119–#124. Already fixed: the foreign-post delete
through `DELETE /api/v1/posts/:id`, in PR #118 (merged). W2.D3 and W3.E6
test the ownership check as a normal, non-skipped test.
