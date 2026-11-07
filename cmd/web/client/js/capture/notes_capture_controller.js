import { Controller } from "@hotwired/stimulus"
import { Capture } from "../notes/capture.js"

// The capture page (/controls/notes/new): Dictate, Photo, Photo + dictate and
// Type, and the focus chip's Stop. It is part of the page's own bundle
// (../capture.js), not of main.js.
export default class extends Controller {
  static targets = ["tap", "modes", "camera", "recorder", "timer", "limit", "recHint", "typer", "text", "error", "card", "cardWhat", "cardStatus"]
  static values = { start: String, maxSeconds: Number, maxDimension: Number, csrf: String, shared: Object, worker: String }

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
  stopFocus() { this.impl.stopFocus() }
}
