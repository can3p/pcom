import { Controller } from "@hotwired/stimulus"
import { runAction } from "../lib"

// The post editor's Notes panel: opens beside the editor, loads the notes
// fragment, and puts a note's text into the post at the cursor or at its end.
// A note counts as used once the post has an id; until the first autosave
// gives it one the ids wait here.
export default class extends Controller {
  static targets = ["panel", "body", "button"]
  static values = { url: String }

  connect() {
    this.pending = new Set()
    // the action goes out as JSON from the panel: json-enc on the whole
    // controller element would also reach the post form
    this.panelTarget.setAttribute("hx-ext", "json-enc")
    this.settled = () => this.flush()
    document.addEventListener("htmx:afterSettle", this.settled)
  }

  disconnect() {
    document.removeEventListener("htmx:afterSettle", this.settled)
  }

  toggle() {
    if (this.panelTarget.hidden) {
      this.open()
    } else {
      this.close()
    }
  }

  open() {
    this.panelTarget.hidden = false
    this.element.classList.add("np-open")
    this.buttonTargets.forEach((b) => b.setAttribute("aria-expanded", "true"))

    if (!this.bodyTarget.hasChildNodes()) {
      window.htmx.ajax("GET", this.urlValue, { target: this.bodyTarget, swap: "innerHTML" })
    }
  }

  close() {
    if (!this.hasPanelTarget) {
      return
    }

    this.panelTarget.hidden = true
    this.element.classList.remove("np-open")
    this.buttonTargets.forEach((b) => b.setAttribute("aria-expanded", "false"))
  }

  insert(event) {
    this.put(event, false)
  }

  append(event) {
    this.put(event, true)
  }

  put(event, atEnd) {
    const { text, noteId } = event.currentTarget.dataset
    const editor = this.application.getControllerForElementAndIdentifier(
      this.element.querySelector("[data-controller~=mdeditor]"),
      "mdeditor",
    )

    editor.insertText(text, { atEnd })
    // save now rather than after the autosave delay
    this.element.querySelector("form")?.dispatchEvent(new CustomEvent("postform:save"))
    this.pending.add(noteId)
    this.flush()
  }

  // records the notes put into the post, once it has an id
  flush() {
    const postId = this.element.querySelector("input[name=post_id]")?.value

    if (!postId || this.pending.size === 0) {
      return
    }

    const ids = [...this.pending]
    this.pending.clear()

    for (const noteId of ids) {
      runAction("note_mark_used", {}, { noteId, postId }, this.panelTarget).catch((e) => {
        console.warn("note_mark_used failed", e)
      })
    }
  }
}
