import { Controller } from "@hotwired/stimulus"

// The author's Comments | Notes toggle of a post page. Comments show first, a
// draft (which has none) opens on its notes, and the choice is kept per
// browser.
export default class extends Controller {
  static targets = ["tab", "pane"]
  static values = { draft: Boolean }

  connect() {
    let pane

    try { pane = localStorage.postPane } catch { /* storage blocked */ }

    this.show(this.draftValue ? "notes" : pane)
  }

  select(event) {
    const pane = event.currentTarget.dataset.pane

    this.show(pane)

    try { localStorage.postPane = pane } catch { /* storage blocked */ }
  }

  show(name) {
    name = name === "notes" ? name : "comments"

    this.tabTargets.forEach((t) => t.setAttribute("aria-selected", t.dataset.pane === name))
    this.paneTargets.forEach((p) => { p.hidden = p.dataset.pane !== name })

    // the Notes tab fetches the notes the first time it is told to
    if (name === "notes") window.htmx.trigger(this.tabTargets.find((t) => t.dataset.pane === name), "notes:show")
  }
}
