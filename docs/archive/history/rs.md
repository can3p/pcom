# RS — Repositories and services, thin handlers (done 2026-09-30)

Built: every query now lives in `pkg/repo` (methods on `*repo.Store`, with
`Tx` joining an open transaction, `Using(exec)` for legacy callers and
factories, `SendMail` so mail is queued in the change's transaction), every
rule in `pkg/service/{accounts,connections,feeds,graph,media,posts,reading,shares}`,
wired through `pkg/service/registry`. Handlers bind, call one service method
and render; the service errors map to responses in one place each
(`ginhelpers.Status`/`HTMLError`, `actionMessage`). `pkg/arch` type-checks
every package and flags queries, ORM imports and database handles passed to
other code outside `pkg/repo`; its allowlist went from 64 files to none.
`pkg/userops`, `feedops.go`, `pkg/web/func.go` and `media.HandleUpload` are
gone. `docs/architecture.md` describes the result.

How it ran: step 0 (coordinator) built the contracts and the shares slice,
extracted the connection graph that three areas share, and split the mixed
handler and page files by area with a line-multiset-checked script, so the
six step 1 tasks owned disjoint files. Each task ran in its own git
worktree; the coordinator merged by cherry-pick. Step 2 (one mid task)
removed the bridges between areas and merged the per-area repository files.

What turned out wrong:
- Agent worktrees start from `origin/master`, not the unpushed wave branch:
  the first dispatch of all six tasks came back BLOCKED with no edits.
- Audits found something in five of seven tasks: deleted tests and changed
  upload texts (L6), weak new tests (L1, L5), no service-level authorization
  tests (L3: removing the owner filter from delete failed only e2e), and
  failures that no longer panicked, so no 500 and no admin mail (L4). All
  were fixed before merging.
- One audit reported the #118 hole reopened. It was the coordinator's
  mutation check, read during the minutes the owner filter was removed.
- A line-union resolver for shared files is safe for imports and struct
  fields, not for code: it deduplicated `}` in `func_test.go`.
- The E2E visibility tests don't catch direct-only posts leaking to
  second-degree viewers on a user's home page; only the privacy matrix
  does.
- `make cover-check` passed on stale `.cover/` data until the close. A
  fresh `make cover` showed the total at 79.4% and `pkg/repo` at 9%.
  Per-package instrumentation didn't count code a service test reaches in
  another package. `make cover` now uses `-coverpkg` over every pcom
  package (total 85.7%), and the floors follow. One `make cover` run
  failed on a test that three reruns didn't reproduce; watch for it in CI.

Behavior changes, accepted: a database failure on the public RSS feed after
the user lookup answers 500 (was 404); API validation errors answer 400
(was 500); dismissing an unknown RSS item and commenting on a missing post
say "not found" instead of the raw "sql: no rows" text; an API key whose
user fails to load now fails the request instead of continuing without a
user; form saves run the service's own transaction instead of joining
gogo's, so an accepted invite stays if starting the session fails. The
prompt service now checks the direct connection itself (the form did).

Left out: moving the page builders from `pkg/web` next to their handlers in
`pkg/web/app` (a package merge touching the privacy matrix; not needed for
the layering, left for a later wave). Factory users still log in through
the legacy password hash (`HashUserPwd`), which real users no longer take.
The dbsender mail queue is exempt from the arch test, like pgsession, until
R3/R5 change gogo's `sender.Sender`.

**Cost.** 1 coordinator session (207 turns, peak 391k context, 208k of tool
results; flags: `cat`×11, raw `go build`/`go test`×27) and 20 subagents
(about 760 turns): six area tasks (L1 strong, L2–L5 mid, L6 cheap), five
fix rounds, step 2 (mid, resumed once after a rate limit), six strong
assertion audits and one re-audit, plus six first dispatches that stopped at
once on the worktree base. Skills: wave-run and wave-close once each; the
subagents' model-shape and test-failure use doesn't show in the stats. Against R1 (69
coordinator turns, 145k peak, about 64 subagent turns), RS cost about ten
times as much, as expected for a wave that rewrote every area rather than
moving it; the coordinator's peak came from merging six worktrees in one
session.
