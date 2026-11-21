import { Controller } from "@hotwired/stimulus"

// The notes list and a note's edit page. This stub is all that every page
// downloads; the rest (../notes/page.js) is a separate chunk fetched where
// the controller appears.
export default class extends Controller {
  static values = { audioUrl: String }

  static targets = [
    "pick", "bar", "count", "tagButton", "sourceButton", "deleteButton",
    "capture", "captureTags", "uploadStatus",
  ]

  async connect() {
    this.gone = false

    const { NotesPage } = await import("../notes/page.js")

    if (this.gone) return

    this.impl = new NotesPage(this)
    this.impl.refresh()
  }

  disconnect() {
    this.gone = true
    this.impl = null
  }

  // a note replaced in place comes back unpicked, so the bar follows the
  // boxes there are
  select() { this.impl?.refresh() }
  pickTargetConnected() { this.impl?.refresh() }
  pickTargetDisconnected() { this.impl?.refresh() }
  captureKey(event) { this.impl?.captureKey(event) }
  addTag(event) { this.impl?.addTag(event) }
  uploadRecording(event) { this.impl?.uploadRecording(event) }
}
