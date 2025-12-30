# Agent Context for PCOM Project

## Frontend Architecture

### Technology Stack
- **htmx** - For AJAX requests and smooth page transitions
- **Stimulus.js** - For JavaScript controllers
- **Go Templates** - Server-side HTML rendering
- **Bootstrap** - CSS framework

### Key Implementation Details

#### Page Transitions with htmx
- The `<body>` tag uses `hx-boost="true"` to enable smooth page transitions
- Located in: `/Users/dima/code/pcom/cmd/web/client/html/header.html:22`
- The `hx-ext="head-support"` extension is enabled for logged-in users to automatically merge `<head>` elements during navigation
- Extension imported in: `/Users/dima/code/pcom/cmd/web/client/js/index.js`

#### Head Element Updates
During page transitions, the following `<head>` elements are automatically updated:
- `<title>` tag
- Meta tags (description, keywords, og:* properties)
- Dynamic stylesheets (e.g., user-specific styles)
- RSS feed links
- Other page-specific head content

This is handled by the `htmx-ext-head-support` extension.

#### Template Structure
- Each page template includes `{{ template "header.html" . }}` at the top
- Each page template includes `{{ template "footer.html" . }}` at the bottom
- Header contains: `<html>`, `<head>`, opening `<body>` tag, and navigation
- Footer contains: closing page container divs, toast container, scripts, closing `</body>` and `</html>` tags

#### CSRF Protection
- CSRF token is passed via htmx headers for logged-in users
- Configured in the `<body>` tag: `hx-headers='{"X-CSRFToken": "{{ .User.CSRFToken }}"}'`

#### Stimulus Controllers
Located in: `/Users/dima/code/pcom/cmd/web/client/js/controllers/`
- `action_controller.js` - Generic action handling
- `clipboard_controller.js` - Clipboard operations
- `collapse_controller.js` - Bootstrap collapse handling
- `commentform_controller.js` - Comment form interactions
- `confirm_controller.js` - Confirmation dialogs
- `gallery_controller.js` - Image gallery
- `mdeditor_controller.js` - Markdown editor
- `selfsubmit_controller.js` - Auto-submit forms
- `spoiler_controller.js` - Spoiler content
- `toast_controller.js` - Toast notifications
- `toaster_controller.js` - Toast container management
- `toggle_controller.js` - Toggle UI elements

#### htmx Configuration
In `/Users/dima/code/pcom/cmd/web/client/js/index.js`:
- `htmx.config.includeIndicatorStyles = false` - CSP compliance
- `htmx.config.allowScriptTags = false` - Security and Turbo-like behavior

#### Special Cases
- Some links use `hx-boost="false"` to disable boosting for:
  - Direct comment links (anchor links)
  - Export functionality (file downloads)
  - External links

## File Locations
- HTML Templates: `/Users/dima/code/pcom/cmd/web/client/html/`
- JavaScript: `/Users/dima/code/pcom/cmd/web/client/js/`
- Main JS entry: `/Users/dima/code/pcom/cmd/web/client/js/index.js`
