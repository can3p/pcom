import { Controller } from "@hotwired/stimulus"

// The author's Comments | Notes toggle of a post page. The tab is in the
// address (#comments or #notes): /posts/<id>#notes shows the notes, which are fetched when first
// shown. A draft, which has no comments, opens on its notes without a hash.
export default class extends Controller {
  static targets = ["tab", "pane"]
  static values = { draft: Boolean }

  connect() {
    this.show(location.hash === "#notes" || (this.draftValue && !location.hash) ? "notes" : "comments")
  }

  select(event) {
    const name = event.currentTarget.dataset.pane

    this.show(name)
    history.replaceState(0, "", "#" + name)
  }

  show(name) {
    this.tabTargets.forEach((t) => t.setAttribute("aria-selected", t.dataset.pane === name))
    this.paneTargets.forEach((p) => { p.hidden = p.dataset.pane !== name })

    // the notes' url is spent once they are fetched
    const pane = this.paneTargets.find((p) => p.dataset.url)

    if (name === "notes" && pane) {
      window.htmx.ajax("GET", pane.dataset.url, { target: pane, swap: "innerHTML" })
      delete pane.dataset.url
    }
  }
}
