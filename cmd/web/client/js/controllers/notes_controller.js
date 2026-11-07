import { Controller } from "@hotwired/stimulus"

// The notes list: a selection of notes, the Tag, Source and Delete buttons
// that act on it, and Ctrl+Enter in the quick-capture box. The buttons are
// action controllers; this one hands them the selected ids and the text their
// confirmation shows.
export default class extends Controller {
  static targets = ["pick", "stackPick", "bar", "count", "tagButton", "sourceButton", "deleteButton", "capture"]

  select() {
    this.refresh()
  }

  selectStack(event) {
    const stack = event.target.dataset.stack

    for (const box of this.pickTargets) {
      if (box.dataset.stack === stack) {
        box.checked = event.target.checked
      }
    }

    this.refresh()
  }

  refresh() {
    const picked = this.pickTargets.filter((box) => box.checked)
    const recordings = picked.filter((box) => box.dataset.recording === "1").length
    const ids = picked.map((box) => box.value).join(",")

    // a stack's box shows whether all its notes are picked
    for (const head of this.stackPickTargets) {
      const own = this.pickTargets.filter((box) => box.dataset.stack === head.dataset.stack)
      head.checked = own.length > 0 && own.every((box) => box.checked)
    }

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
}
