import { runAction, reportError } from "../lib"

// The post editor's Notes tab, loaded by controllers/notes_picker_controller.js
// in the editor only, so the main bundle stays small. A note counts as used
// once the post has an id; until the post is saved as a draft the ids wait
// here, and in the form's hidden note_ids, which the new post's scratch pad
// keeps, so that they survive a reload.
export class NotesPicker {
  constructor(c) {
    this.c = c
    this.ids = this.c.element.querySelector("input[name=note_ids]")
    this.pending = new Set((this.ids?.value || "").split(",").filter((id) => id !== ""))
    // the action goes out as JSON from the panel: json-enc on the whole
    // controller element would also reach the post form
    this.c.panelTarget.setAttribute("hx-ext", "json-enc")
    this.settled = () => {
      this.learnPost()
      this.flush()
    }
    document.addEventListener("htmx:afterSettle", this.settled)
    this.post = this.postId()
  }

  disconnect() {
    document.removeEventListener("htmx:afterSettle", this.settled)
  }

  addTag(event) {
    const input = this.c.panelTarget.querySelector(".np-search input[name=q]")
    const tag = `#${event.currentTarget.dataset.tag}`
    const words = input.value.split(/\s+/).filter((w) => w !== "")

    if (!words.some((w) => w.toLowerCase() === tag.toLowerCase())) {
      words.push(tag)
    }

    input.value = words.join(" ")
    input.form.requestSubmit()
  }

  // Attach and Detach on a card of a saved post: the card follows at once.
  // A note that a tag brings is hidden from the post rather than detached.
  toggle(event) {
    if (event.currentTarget.dataset.tag) return this.attachTag(event)

    const button = event.currentTarget
    const { noteId, attached, via } = button.dataset
    const on = attached !== "1"
    const name = on ? "note_attach" : via === "1" ? "note_exclude" : "note_detach"

    runAction(name, {}, { noteId, postId: this.postId() }, this.c.panelTarget).then(() => this.attached(noteId, on), reportError)
  }

  // the card of a note shows whether it belongs to the post
  attached(noteId, on) {
    const card = this.c.panelTarget.querySelector(`#np-note-${noteId}`)
    const button = card?.querySelector("[data-action='notes-picker#toggle']")
    if (!button) return

    button.dataset.attached = on ? "1" : "0"
    button.dataset.via = "0"
    button.setAttribute("aria-label", on ? "Detach" : "Attach")
    button.title = on ? "Detach from this post" : "Attach to this post"

    const mark = card.querySelector(".np-attached")
    if (mark) mark.hidden = !on
  }

  // Attach all notes tagged X: the tag stays attached, so what the author
  // tags later joins the post, which the confirmation says first.
  attachTag(event) {
    const button = event.currentTarget
    const { tag, prompt } = button.dataset

    if (!confirm(prompt)) return

    runAction("note_attach_tag", {}, { tag, postId: this.postId() }, this.c.panelTarget).then(() => {
      const done = document.createElement("span")
      done.className = "meta"
      done.textContent = `#${tag} is attached to this post.`
      button.replaceWith(done)
    }, reportError)
  }

  postId() {
    return this.c.element.querySelector("input[name=post_id]")?.value
  }

  // A new post gets its id when the pad is saved as a draft; the panel then
  // loads again, with the controls that attach notes on.
  learnPost() {
    const id = this.postId()
    if (!id || id === this.post) return

    this.post = id

    const form = this.c.panelTarget.querySelector(".np-search")
    const body = this.c.panelTarget.querySelector(".np-body")
    let hidden = form.querySelector("input[name=post]")

    if (!hidden) {
      hidden = document.createElement("input")
      hidden.type = "hidden"
      hidden.name = "post"
      form.append(hidden)
    }

    hidden.value = id
    body.setAttribute("hx-vals", JSON.stringify({ post: id }))

    window.htmx.ajax("GET", form.getAttribute("action"), {
      source: form, target: body, swap: "innerHTML", values: { q: form.elements.q.value, post: id },
    })
  }

  insert(event) {
    this.put(event)
  }

  put(event) {
    const { text, noteId } = event.currentTarget.dataset
    const editor = this.c.application.getControllerForElementAndIdentifier(
      this.c.element.querySelector("[data-controller~=mdeditor]"),
      "mdeditor",
    )

    editor.insertText(text)
    // the save below carries the ids
    this.pending.add(noteId)
    this.keep()
    // save now rather than after the autosave delay
    this.c.element.querySelector("form")?.dispatchEvent(new CustomEvent("postform:save"))
    this.flush()
  }

  // keep puts the pending ids into the form, where a new post saves them.
  keep() {
    if (this.ids) this.ids.value = [...this.pending].join(",")
  }

  flush() {
    const postId = this.c.element.querySelector("input[name=post_id]")?.value

    if (!postId || this.pending.size === 0) {
      return
    }

    const ids = [...this.pending]
    this.pending.clear()
    this.keep()

    for (const noteId of ids) {
      // marking a note as used attaches it to the post
      runAction("note_mark_used", {}, { noteId, postId }, this.c.panelTarget).then(() => this.attached(noteId, true), (e) => {
        console.warn("note_mark_used failed", e)
      })
    }
  }
}
