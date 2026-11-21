# The pcom checklist

Four questions, asked of a diff (skill `pcom-review`) or of a proposal before any code (skill `pcom-critic`).
Each item names what to look for; the reason is in the doc it points at.

## 1. Is it idiomatic, and done once?

Use what pcom already has before writing something new:

- **An HTML form is a `pkg/forms` type** run by `DefaultHandler` ("Forms" in `docs/architecture.md`). Red flags:
  a handler reading `c.PostForm`, a page type mirroring a form's input and errors (`Title`, `Error`,
  `Saved`), a hand-written "answer with an error partial" helper, a check in the handler.
- **A button that acts without a form** posts JSON through the action controller to `jsonAction`
  (skill `frontend-htmx`). Not a bespoke `fetch` or a form for one click.
- **A URL is built with `link`** in templates and `links.Link` in Go, `hx-*` and `action` attributes included.
- **Layers:** handlers bind, call a service, render; rules and authorization in `pkg/service/<area>`; SQL in
  `pkg/repo` (AGENTS.md). A query, an ownership check or a business rule in a handler is a finding.
- **A limit or tunable is a setting** in `pkg/config` passed as a service option (AGENTS.md "No magic numbers").
- **Components and tokens** from the design system, not new CSS for a thing `_components.scss` has.
- **One mechanism per job.** Look for the same logic twice: two helpers that map service errors, two copies
  of a JS module (a service worker and its page), a parser in Go and JS with no shared test table, two
  structs for one input.
- **No dead flexibility.** An option nobody passes as anything but its default (`WithX(false)` that never
  happens), pointer fields every caller sets, a "this or that, that wins" pair of fields, an enum with one
  reader. Remove them.
- **The schema says what is true.** A foreign key that can point at the wrong kind of row, a column whose
  meaning depends on another column, a rule only the service enforces when a constraint could.
- **Comments** state the data a partial takes or a non-obvious why; they don't restate the markup or code.

## 2. Does it behave like the rest of pcom?

Compare with the nearest existing screen or flow and name it:

- **Form results** show in a `.status` next to the form's button; field errors under the field
  (`.field-error`, `aria-invalid`); never a toast. A toast is for an action without a form.
- **Saving:** in place by default; `HX-Refresh` only when the page must change shape; an autosaving form
  targets only its status so the field being typed in is never swapped.
- **Wording:** "Saved", "Not saved: <why>", the same verbs as existing controls (Delete, Edit, Cancel).
- **Navigation:** boosted links; a page with its own bundle answers a boosted request with `HX-Redirect`.
- **Not found vs forbidden:** somebody else's thing is not found, as everywhere else.
- **Empty and error states** like the neighbouring pages; keyboard and screen-reader behaviour the same.
- **Capabilities, not template conditions**, decide what a viewer may do.

## 3. Critical gaps

Only what would hurt a user or the data:

- **Authorization** on every new route and service method (owner checks; another user's id is refused).
- **CSRF** on every POST; an exemption needs the written argument next to the route (skill `frontend-htmx`).
- **Data loss or corruption:** a failed step leaving rows or files behind, a race on a unique key, a delete
  that misses dependants (files in the bucket, jobs), a migration that can't run on existing data.
- **Limits** on anything a user can send (sizes, counts, rates).
- **Tests:** every new route in the guard table (`e2e/guards_test.go`); behaviour tested at the cheapest
  level that proves it (`docs/testing.md`); a browser test for JS that matters.
- **Docs** made untrue by the change are updated in the same diff (AGENTS.md).

## 4. Can it be simpler and still do the job?

- Could an existing mechanism (above) replace new code?
- Is there a path, type, option, state or table that could go without losing a requirement?
- Is a client-side piece needed, or would a server-rendered htmx fragment do?
- Is the generality used? Code for a case no caller has is a finding.
- Would a smaller change land the same user-visible result?

## Weighing a finding

Report a finding only after checking it against the code (open the file, follow the call). For each:
the lens (1–4), `path:line`, what is wrong in one sentence, the concrete fix, and its cost (small / medium /
large). Rank by harm first (gaps), then by how much simpler or more consistent the fix makes the code.
Leave out taste with no reason from this list or the docs.
