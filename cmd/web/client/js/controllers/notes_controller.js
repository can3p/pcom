import { Controller } from "@hotwired/stimulus"

// The notes list and a note's edit page. The selection bar's buttons are
// action controllers; this one hands them the selected ids and the delete
// confirmation's text.
export default class extends Controller {
  static values = { audioUrl: String }

  static targets = [
    "pick", "bar", "count", "tagButton", "sourceButton", "deleteButton",
    "capture", "captureTags", "uploadStatus",
  ]

  select() {
    this.refresh()
  }

  // A note replaced in place (its transcript came in) comes back unpicked,
  // so the bar follows the boxes there are.
  pickTargetConnected() {
    this.refresh()
  }

  pickTargetDisconnected() {
    this.refresh()
  }

  refresh() {
    // the manage and edit pages have no selection bar
    if (!this.hasBarTarget) return

    const picked = this.pickTargets.filter((box) => box.checked)
    const recordings = picked.filter((box) => box.dataset.recording === "1").length
    const ids = picked.map((box) => box.value).join(",")

    this.barTarget.hidden = picked.length === 0
    this.countTarget.textContent = `${picked.length} selected`

    for (const button of [this.tagButtonTarget, this.sourceButtonTarget, this.deleteButtonTarget]) {
      button.dataset.ids = ids
    }

    const notes = picked.length === 1 ? "1 note" : `${picked.length} notes`
    const rec = recordings === 0 ? "" : ` and ${recordings === 1 ? "1 recording" : `${recordings} recordings`}`

    this.deleteButtonTarget.setAttribute(
      "data-action-prompt-value",
      `Delete ${notes}${rec}? This can't be undone.`,
    )
  }

  captureKey(event) {
    if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
      event.preventDefault()
      this.captureTarget.requestSubmit()
    }
  }

  addTag(event) {
    const tag = event.currentTarget.dataset.tag
    const input = this.captureTagsTarget
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
      const response = await fetch(this.audioUrlValue, {
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
    if (!this.hasUploadStatusTarget) return

    const span = document.createElement("span")
    span.className = failed ? "status err" : "status"
    span.setAttribute("role", "status")
    span.textContent = text
    this.uploadStatusTarget.replaceChildren(span)
  }
}

function csrfHeaders() {
  try {
    return JSON.parse(document.body.getAttribute("hx-headers") || "{}")
  } catch {
    return {}
  }
}
