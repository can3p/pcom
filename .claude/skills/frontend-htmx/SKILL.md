---
name: frontend-htmx
description: Conventions for pcom's frontend - Go HTML templates, htmx (hx-boost, json-enc, response headers), Stimulus controllers including the generic action controller, CSRF, Bootstrap dark-mode SCSS, the renderHumanTime helper, and checking a change with the browser test suite (make test-ui). Use before changing or adding anything under cmd/web/client/ (html, js, scss) or a handler that returns htmx responses.
---

# Frontend (templates, JS, styles)

## Frontend Architecture

### Technology Stack
- **htmx** - AJAX requests and page transitions
- **Stimulus.js** - JavaScript controllers
- **Go Templates** - Server-side HTML rendering
- **Bootstrap** - CSS framework

### htmx Configuration
Located in `cmd/web/client/js/index.js`:
- `htmx.config.includeIndicatorStyles = false` - CSP compliance
- `htmx.config.allowScriptTags = false` - Security and Turbo-like behavior
- `hx-boost="true"` on `<body>` enables smooth page transitions
- `hx-ext="head-support"` auto-merges `<head>` elements during navigation
- `json-enc` extension for JSON payloads (sets `Content-Type: application/json`, stringifies parameters)

### Template Structure
- Each page includes `{{ template "header.html" . }}` (contains `<html>`, `<head>`, `<body>`, nav)
- Each page includes `{{ template "footer.html" . }}` (closing tags, scripts)

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
  add a `data-testid` to the template. That is fine in frontend work, but not in test waves, which don't
  change templates.
- Wait with auto-waiting locator assertions, never sleeps. After an htmx action, assert on the swapped
  element, not on the network.

Handlers behind htmx endpoints are thin (see the layering note in `AGENTS.md`): they call a service and set
the response headers. Don't put queries into a handler to feed a template.

## Dark Mode Styling

### Architecture
Located in `cmd/web/client/scss/_dark-mode.scss`:
- Uses Bootstrap 5.3+ color modes with `data-bs-theme="dark"` and `prefers-color-scheme` media query
- All styles defined in `@mixin dark-mode-styles` for reusability

### CSS Variable Override Pattern
**Override Bootstrap component variables by scoping them within component selectors:**
```scss
.list-group {
  --bs-list-group-bg: #2d2d2d;
  --bs-list-group-border-color: #404040;
  --bs-list-group-color: var(--text-color);
}
```

**Do NOT define ad-hoc colors directly on elements** - always use Bootstrap's CSS variables to ensure proper inheritance and theming.

## Date/Time Rendering

Use `renderHumanTime` template helper to display timestamps with relative time and full timestamp on hover:

```html
{{ renderHumanTime .CreatedAt $.User.DBUser }}
```

**Output**: `<span title="Mon, 15 Jan 2024 12:30">5 minutes ago</span>`

- First argument: `time.Time` value to display
- Second argument: `*core.User` for timezone localization (can be nil for UTC)
- Implementation: `pkg/util/date`
