import { Controller } from "@hotwired/stimulus"

// The notes list: a selection of notes, the Tag, Source and Delete buttons
// that act on it, and Ctrl+Enter in the quick-capture box. The buttons are
// action controllers; this one hands them the selected ids and the text their
// confirmation shows.
export default class extends Controller {
  static targets = ["pick", "bar", "count", "tagButton", "sourceButton", "deleteButton", "capture"]

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
    // the manage page has no selection bar
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
}
