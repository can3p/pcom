import { Controller } from "@hotwired/stimulus"
import { Capture } from "../notes/capture.js"

// In the capture page's own bundle; the work is in ../notes/capture.js. In
// the audio value, ":id" stands for the note.
export default class extends Controller {
  static targets = ["tap", "modes", "camera", "recorder", "timer", "limit", "recHint", "typer", "text", "error", "card", "cardWhat", "cardStatus"]
  static values = { start: String, maxSeconds: Number, maxDimension: Number, csrf: String, shared: Object, worker: String, scope: String, delete: String, audio: String }

  connect() {
    this.impl = new Capture(this)
  }

  disconnect() {
    this.impl.disconnect()
  }

  dictate() { this.impl.dictate() }
  photo() { this.impl.photo(false) }
  photoDictate() { this.impl.photo(true) }
  photoTaken() { this.impl.photoTaken() }
  type() { this.impl.type() }
  cancelText() { this.impl.cancelText() }
  saveText(e) { e.preventDefault(); this.impl.saveText() }
  stop() { this.impl.stop() }
  tapStart() { this.impl.tapStart() }
  deleteLast() { this.impl.deleteLast() }
}
