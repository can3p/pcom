import { Controller } from "@hotwired/stimulus"

// The post editor's Notes tab. This stub is all that every page downloads;
// the rest (../notes/picker.js) is a separate chunk fetched in the editor.
export default class extends Controller {
  static targets = ["panel"]

  async connect() {
    this.gone = false

    const { NotesPicker } = await import("../notes/picker.js")

    if (!this.gone) this.impl = new NotesPicker(this)
  }

  disconnect() {
    this.gone = true
    this.impl?.disconnect()
    this.impl = null
  }

  addTag(event) { this.impl?.addTag(event) }
  insert(event) { this.impl?.insert(event) }
  append(event) { this.impl?.append(event) }
}
