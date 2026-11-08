import { Controller } from "@hotwired/stimulus"
import { runAction } from "../lib"

// The cards of the post editor's Notes tab: puts a note's text into the post
// at the cursor or at its end. The tab itself (ed_side_controller.js) loads
// the list and the search; a note counts as used once the post has an id,
// and until the first autosave gives it one the ids wait here.
export default class extends Controller {
  static targets = ["panel"]

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
