import { runAction, reloadPage, reportError } from "../lib"

// The notes list and a note's edit page, loaded by
// controllers/notes_controller.js on those pages only, so the main bundle
// stays small. The selection bar's buttons are action controllers; this
// hands them the selected ids and the delete confirmation's text.
export class NotesPage {
  constructor(c) {
    this.c = c
  }

  refresh() {
    // the manage and edit pages have no selection bar
    if (!this.c.hasBarTarget) return

    const picked = this.c.pickTargets.filter((box) => box.checked)
    const recordings = picked.filter((box) => box.dataset.recording === "1").length
    const ids = picked.map((box) => box.value).join(",")

    this.c.barTarget.hidden = picked.length === 0
    this.c.countTarget.textContent = `${picked.length} selected`

    for (const button of [this.c.tagButtonTarget, this.c.sourceButtonTarget, this.c.deleteButtonTarget]) {
      button.dataset.ids = ids
    }

    const notes = picked.length === 1 ? "1 note" : `${picked.length} notes`
    const rec = recordings === 0 ? "" : ` and ${recordings === 1 ? "1 recording" : `${recordings} recordings`}`

    this.c.deleteButtonTarget.setAttribute(
      "data-action-prompt-value",
      `Delete ${notes}${rec}? This can't be undone.`,
    )
  }

  // Attach to post…, or Attach all notes tagged X (a button with data-tag): the note's own select names the post. The list comes
  // back with the note saying which posts it is attached to.
  attach(event) {
    if (event.currentTarget.dataset.tag) return this.attachTag(event)

    const button = event.currentTarget
    const postId = button.closest(".note-attach-form").querySelector("select").value

    runAction("note_attach", {}, { noteId: button.dataset.noteId, postId }, button).then(reloadPage, reportError)
  }

  // Attach all notes tagged X to the post chosen above the list, after the
  // confirmation the server worded.
  attachTag(event) {
    const button = event.currentTarget
    const { tag, prompt } = button.dataset

    if (!confirm(prompt)) return

    const postId = button.closest(".nt-attach").querySelector("select").value

    runAction("note_attach_tag", {}, { tag, postId }, button).then(reloadPage, reportError)
  }

  captureKey(event) {
    if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
      event.preventDefault()
      this.c.captureTarget.requestSubmit()
    }
  }

  addTag(event) {
    const tag = event.currentTarget.dataset.tag
    const input = this.c.captureTagsTarget
    const tags = input.value.split(/[\s,]+/).filter((t) => t !== "")

    if (!tags.some((t) => t.replace(/^#+/, "").toLowerCase() === tag)) {
      tags.push(tag)
    }

    input.value = tags.join(" ")
    input.focus()
  }

  // a new note, or the recording of the edit page's note (data-note-id)
  async uploadRecording(event) {
    const input = event.currentTarget
    const file = input.files?.[0]
    if (!file) return

    const form = new FormData()
    form.append("file", file)
    if (input.dataset.noteId) form.append("note_id", input.dataset.noteId)

    this.uploadStatus("Uploading…")

    try {
      const response = await fetch(this.c.audioUrlValue, {
        method: "POST",
        headers: csrfHeaders(),
        body: form,
      })

      if (!response.ok) {
        const body = await response.json().catch(() => ({}))
        throw new Error(body.explanation || "The recording could not be uploaded.")
      }

      window.location.reload()
    } catch (err) {
      this.uploadStatus(err.message, true)
    } finally {
      input.value = ""
    }
  }

  uploadStatus(text, failed = false) {
    if (!this.c.hasUploadStatusTarget) return

    const span = document.createElement("span")
    span.className = failed ? "status err" : "status"
    span.setAttribute("role", "status")
    span.textContent = text
    this.c.uploadStatusTarget.replaceChildren(span)
  }
}

function csrfHeaders() {
  try {
    return JSON.parse(document.body.getAttribute("hx-headers") || "{}")
  } catch {
    return {}
  }
}
