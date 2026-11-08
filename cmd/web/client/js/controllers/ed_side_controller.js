import { Controller } from "@hotwired/stimulus"

const KEY = "pcom.editor.side"

// The post editor's sidebar: a tab for the grammar check and style review
// (checks) and one for the notes (notes), and a collapse to a thin strip. The
// state is remembered per browser (collapsed until chosen otherwise); the editor redraws itself after a failed
// save, and the new controller reads the state back.
export default class extends Controller {
  static targets = ["tab", "pane", "toggle", "body"]

  connect() {
    const saved = this.load()
    const tabs = this.tabTargets.map((t) => t.dataset.tab)

    this.tab = tabs.includes(saved.tab) ? saved.tab : tabs[0]
    // a browser that has not chosen yet starts with the strip: the sidebar opens when asked for
    this.collapsed = saved.collapsed !== false
    this.render()
  }

  select(event) {
    this.tab = event.currentTarget.dataset.tab
    this.render()
    this.save()
  }

  toggle() {
    this.collapsed = !this.collapsed
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

    this.toggleTarget.setAttribute("aria-expanded", String(!this.collapsed))

    const label = this.collapsed ? "Expand the sidebar" : "Collapse the sidebar"
    this.toggleTarget.setAttribute("aria-label", label)
    this.toggleTarget.title = label

    this.tabTargets.forEach((t) => {
      const on = t.dataset.tab === this.tab

      t.setAttribute("aria-selected", String(on))
      t.tabIndex = on ? 0 : -1
    })
    this.paneTargets.forEach((p) => {
      p.hidden = p.dataset.tab !== this.tab
    })
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
