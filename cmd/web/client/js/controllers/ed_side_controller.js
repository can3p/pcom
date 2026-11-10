import { Controller } from "@hotwired/stimulus"

const KEY = "pcom.editor.side"

// The post editor's sidebar. Collapsed, it is a strip level with the editor
// box and as tall, which CSS can't line up across the form: the box is
// measured into --es-strip-top and --es-strip-height whenever either resizes.
// The state is per browser, so the editor redrawn after a failed save reads it
// back. The measures leave the element before htmx snapshots the page for its
// history: restored, a style attribute would break the CSP.
export default class extends Controller {
  static targets = ["tab", "pane", "toggle", "body", "strip"]

  connect() {
    const saved = this.load()
    const tabs = this.tabTargets.map((t) => t.dataset.tab)

    this.tab = tabs.includes(saved.tab) ? saved.tab : tabs[0]
    // a browser that has not chosen yet starts with the strip: the sidebar opens when asked for
    this.collapsed = saved.collapsed !== false
    this.render()

    this.measure = () => this.place()
    this.observer = new ResizeObserver(this.measure)
    this.observer.observe(this.element)
    this.observeBox()
    window.addEventListener("resize", this.measure)
    // htmx takes the snapshot right after the event; then the strip goes back
    this.unplace = () => {
      this.element.removeAttribute("style")
      setTimeout(this.measure, 0)
    }
    document.addEventListener("htmx:beforeHistorySave", this.unplace)
  }

  disconnect() {
    this.observer?.disconnect()
    window.removeEventListener("resize", this.measure)
    document.removeEventListener("htmx:beforeHistorySave", this.unplace)
  }

  select(event) {
    this.tab = event.currentTarget.dataset.tab
    this.render()
    this.save()
  }

  open(event) {
    this.tab = event.currentTarget.dataset.tab
    this.collapsed = false
    this.render()
    this.save()
  }

  collapse() {
    this.collapsed = true
    this.render()
    this.save()
  }

  // the toolbar's Check grammar and Style review
  showChecks() {
    if (!this.tabTargets.some((t) => t.dataset.tab === "checks")) {
      return
    }

    this.tab = "checks"
    this.collapsed = false
    this.render()
    this.save()
  }

  render() {
    this.element.classList.toggle("es-collapsed", this.collapsed)
    this.bodyTarget.hidden = this.collapsed
    this.stripTarget.hidden = !this.collapsed
    this.toggleTarget.setAttribute("aria-expanded", String(!this.collapsed))

    this.tabTargets.forEach((t) => {
      const on = t.dataset.tab === this.tab

      t.setAttribute("aria-selected", String(on))
      t.tabIndex = on ? 0 : -1
    })
    this.paneTargets.forEach((p) => {
      p.hidden = p.dataset.tab !== this.tab
    })

    this.place()
  }

  box() {
    return this.element.querySelector(".ed-box")
  }

  observeBox() {
    const box = this.box()

    if (box && box !== this.observed) {
      this.observer.observe(box)
      this.observed = box
    }
  }

  place() {
    const box = this.box()
    if (!box) return

    if (this.observer) this.observeBox()

    const cols = this.element.getBoundingClientRect()
    const rect = box.getBoundingClientRect()

    this.element.style.setProperty("--es-strip-top", `${Math.max(0, rect.top - cols.top)}px`)
    this.element.style.setProperty("--es-strip-height", `${rect.height}px`)
  }

  load() {
    try {
      return JSON.parse(localStorage.getItem(KEY)) || {}
    } catch {
      return {}
    }
  }

  save() {
    try {
      localStorage.setItem(KEY, JSON.stringify({ tab: this.tab, collapsed: this.collapsed }))
    } catch {
      // private window or blocked storage: the sidebar just forgets
    }
  }
}
