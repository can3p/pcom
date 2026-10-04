---
name: frontend-htmx
description: Conventions for pcom's frontend - Go HTML templates, htmx (hx-boost, json-enc, response headers), Stimulus controllers including the generic action controller, CSRF, the design system (tokens, components, dark mode), the renderHumanTime helper, and checking a change with the browser test suite (make test-ui). Use before changing or adding anything under cmd/web/client/ (html, js, scss) or a handler that returns htmx responses.
---

# Frontend (templates, JS, styles)

## Frontend Architecture

### Technology Stack
- **htmx** - AJAX requests and page transitions
- **Stimulus.js** - JavaScript controllers
- **Go Templates** - Server-side HTML rendering
- **SCSS design system** - pcom's own tokens and components (no CSS framework; see "Design system")

### htmx Configuration
Located in `cmd/web/client/js/index.js`:
- `htmx.config.includeIndicatorStyles = false` - CSP compliance
- `htmx.config.allowScriptTags = false` - Security and Turbo-like behavior
- `hx-boost="true"` on `<body>` enables smooth page transitions
  - Boost hides broken fragment endpoints: a link with its own `hx-target`/`hx-swap` is fetched in place
    even without `hx-get`, and a full-page response pasted into a list still "shows" the new items. A
    "load more" or partial-swap browser test must assert the page structure isn't duplicated (one navbar,
    one heading) and that earlier content stays; prove it by making the handler return the full page.
- `hx-ext="head-support"` auto-merges `<head>` elements during navigation
- `json-enc` extension for JSON payloads (sets `Content-Type: application/json`, stringifies parameters)

### Template Structure
- Each page includes `{{ template "header.html" . }}` (contains `<html>`, `<head>`, `<body>`, nav)
- Each page includes `{{ template "footer.html" . }}` (closing tags, scripts)
- **What a viewer may do is a capability, never a template condition.** Show a control with
  `{{ if .Capabilities.CanX }}` from the view type (`postops.PostCapabilities`,
  `postops.CommentCapabilities`, ...). Don't compare ids (`eq $.User.DBUser.ID .UserID`) or combine
  permissions in the template: add a field computed in Go, next to the rule the service enforces, with a
  unit test. A control and the form it opens use the same capability.

### CSRF Protection
- Token passed via `hx-headers='{"X-CSRFToken": "{{ .User.CSRFToken }}"}'` on `<body>` tag

### Action Controller Pattern
**Location**: `cmd/web/client/js/controllers/action_controller.js`

Generic controller for server actions with confirmation dialogs:
- **Values**: `action`, `prompt`, `promptField`, `skipReload`
- **Behavior**: Calls `/controls/action/{action}` via `runAction()` with JSON payload
- **`connect()`**: Automatically adds `json-enc` extension to element
- **`skipReload`**: When `true`, skips page reload on success (allows htmx response headers to control behavior)

**runAction Implementation** (`cmd/web/client/js/lib.js`):
- Uses `htmx.ajax()` instead of `fetch()` to enable htmx response header interpretation
- Reads `hx-target` and `hx-swap` attributes from element
- Constructs URL as `/controls/action/{name}` and payload from element dataset
- Merges CSRF headers from `<body hx-headers>`
- Returns promise that resolves on success or rejects with error

**Usage Pattern**:
```html
<div id="item-{{ .ID }}">
  <button data-controller="action"
          data-action="action#run"
          data-action-action-value="delete_item"
          data-action-prompt-value="Confirm?"
          data-action-skip-reload-value="true"
          data-id="{{ .ID }}"
          hx-target="#item-{{ .ID }}"
          hx-swap="delete">Delete</button>
</div>
```

Server can control behavior via htmx response headers (`HX-Reswap`, `HX-Redirect`, etc.)

## Checking a frontend change

The browser suite (`e2e/browser`; `docs/testing.md` has the "Browser tests" section with a worked example)
runs the real app with freshly built assets in Chromium. It fails on console errors, uncaught exceptions,
CSP violations and failed or 404/5xx requests on every page, so an inline script or a style without the
nonce shows up without a dedicated test. A test that provokes an error on purpose exempts it with
`browser.Allow`.

- `make test-ui RUN='<TestName>'` while iterating; `HEADED=1 SLOWMO=250` to watch it run. The target runs
  `yarn build` first. Run the whole suite once before committing.
- On a failure, the report prints a trace and a screenshot path. `make ui-trace F=<path>` opens the trace
  (DOM snapshot per step, network, console). Look at the screenshot before reading any HTML.
- A change to a controller, a template's interactive parts or an htmx attribute comes with a browser test in
  the matching `e2e/browser/<area>_test.go`. Assert what the user sees and the DB state, never htmx
  internals (events, `hx-*` attributes, request headers), so the test survives an htmx upgrade and catches
  one that breaks the page. Only server rules that don't need the page's JavaScript (access control,
  guards, API, RSS, headers) belong in the cheaper `e2e/` HTTP tests, which don't imitate htmx.
- Locate by role, label and text (`page.GetByRole("button", …{Name: "Publish"})`). Where that's ambiguous,
  add a `data-testid` to the template.
- Wait with auto-waiting locator assertions, never sleeps. After an htmx action, assert on the swapped
  element, not on the network.

Handlers behind htmx endpoints are thin (see the layering note in `AGENTS.md`): they call a service and set
the response headers. Don't put queries into a handler to feed a template.

## Design system

Source is `cmd/web/client/scss/`; `index.scss` imports the partials. There is no Bootstrap and no icon font.

- **Tokens** live in `_tokens.scss`: colors, spacing, shape and type. A new rule reads a token, never a literal
  color. Light (paper) is the default; the warm dark theme redefines the same tokens under
  `prefers-color-scheme: dark` and under `data-theme="dark"`. Dark mode is only a token override, so a rule
  that uses tokens needs no dark variant.
- **Type**: Golos Text, self-hosted (`@fontsource/golos-text`), weights 400 (text), 600 (labels, buttons) and
  800 (titles) only, Latin and Cyrillic subsets only. Navigation and actions use the system monospace stack.
  Don't add a weight or a subset: the `@font-face` list in `_tokens.scss` and the `rel="preload"` links in
  `header.html` (400 and 800, Latin and Cyrillic) change together.
- **Components** are in `_components.scss`: `.btn` with `.btn-primary`, `.btn-secondary`, `.btn-danger`;
  `.field`; `.meta`; `.acts`; `.item`; `.box`; `.status`; `.nav`; `.toast`; the editor toolbar. Reuse them
  before adding a rule.
- **Item actions**: items (posts, comments, feed items) are not boxed. One `.acts` row of text actions sits
  under each item (comments, reply, edit, share, delete); don't add buttons elsewhere on an item.
- **Form results**: a form saves in place. Its result (`Saved` / `Not saved`) shows in a `.status` next to
  the form's button and field errors show under the field. A form result is never a toast; a toast is only
  for an action without a form.
- **Area styles** live in their own partial: `_posts`, `_settings`, `_editor`, `_auth`, `_controls`,
  `_syntax`. Shared pieces go to `_components.scss`, element defaults to `_base.scss`.
- **User style hooks**: the `us-*` classes in templates are the contract for users' own CSS (listed in
  `docs/guide/settings.md`). Keep them on their elements; other classes may change freely.
- **Guards**: `TestTemplatesAndControllersUseNoBootstrapClasses` and `TestTemplatesKeepUserStyleHooks`
  (`pkg/web/app`) and `TestFirstVisitWeight` (`e2e/browser`: per page CSS 25 KB, fonts 70 KB, JS 50 KB
  gzipped, on the production build that `make test-ui` makes).

## Date/Time Rendering

Use `renderHumanTime` template helper to display timestamps with relative time and full timestamp on hover:

```html
{{ renderHumanTime .CreatedAt $.User.DBUser }}
```

**Output**: `<span title="Mon, 15 Jan 2024 12:30">5 minutes ago</span>`

- First argument: `time.Time` value to display
- Second argument: `*core.User` for timezone localization (can be nil for UTC)
- Implementation: `pkg/util/date`
