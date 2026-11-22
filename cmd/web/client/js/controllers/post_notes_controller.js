import { Controller } from "@hotwired/stimulus"

const KEY = "pcom.post.pane"

// The author's Comments | Notes toggle on a post page. Comments show first;
// the choice is kept per browser, and a draft, which has no comments, opens on
// its notes. The notes are fetched from the server when they are first shown,
// so a page holds none of them until its author asks.
export default class extends Controller {
  static targets = ["tab", "pane", "loading"]
  static values = { url: String, draft: Boolean }

  connect() {
    this.loaded = false
    this.show(this.draftValue ? "notes" : this.remembered())
  }

  select(event) {
    const pane = event.currentTarget.dataset.pane

    this.show(pane)

    try {
      localStorage.setItem(KEY, pane)
    } catch {
      // private window or blocked storage: the choice just isn't kept
    }
  }

  remembered() {
    try {
      return localStorage.getItem(KEY) === "notes" ? "notes" : "comments"
    } catch {
      return "comments"
    }
  }

  show(name) {
    for (const tab of this.tabTargets) {
      tab.setAttribute("aria-selected", String(tab.dataset.pane === name))
      tab.tabIndex = tab.dataset.pane === name ? 0 : -1
    }

    for (const pane of this.paneTargets) {
      pane.hidden = pane.dataset.pane !== name
    }

    if (name === "notes" && !this.loaded) {
      this.loaded = true
      this.load()
    }
  }

  load() {
    const pane = this.paneTargets.find((p) => p.dataset.pane === "notes")

    window.htmx.ajax("GET", this.urlValue, { target: pane, swap: "innerHTML" })
  }
}
