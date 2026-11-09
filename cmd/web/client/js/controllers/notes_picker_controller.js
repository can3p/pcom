import { Controller } from "@hotwired/stimulus"
import { runAction } from "../lib"

// The post editor's Notes tab. A note counts as used once the post has an id;
// until the first autosave gives it one, the ids wait here.
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

  addTag(event) {
    const input = this.panelTarget.querySelector(".np-search input[name=q]")
    const tag = `#${event.currentTarget.dataset.tag}`
    const words = input.value.split(/\s+/).filter((w) => w !== "")

    if (!words.some((w) => w.toLowerCase() === tag.toLowerCase())) {
      words.push(tag)
    }

    input.value = words.join(" ")
    input.form.requestSubmit()
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
