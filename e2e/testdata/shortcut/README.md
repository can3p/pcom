# Recorded iPhone Shortcut requests

The raw requests the "pcom note" and "Quote from photo" Shortcuts send, recorded on the owner's iPhone as
docs/api.md ("The requests of the iPhone Shortcut") describes. Until they are here,
`TestAPINotes_ShortcutRequests` builds the same requests in Go; once they are, it replays these files.

| File | Run |
|---|---|
| `action-button.http` | from the Action button: Record Audio |
| `share-text.http` | from the share sheet with selected text |
| `share-url.http` | from the share sheet with a link |
| `share-images.http` | from the share sheet with two photos |
| `quote-from-photo.http` | "Quote from photo": Take Photo, Extract Text from Image |

Each file is the request as it arrived, headers and body byte for byte, with the `Authorization` value
replaced by `Bearer <key>`.

Recorded with iOS: (not yet)

"Get Contents of URL" with the phone locked: (not yet checked)
