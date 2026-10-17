import { Controller } from "@hotwired/stimulus"

// A side index of the page's sections (the settings page): marks the link of
// the section being read with aria-current="location" and scrolls to a
// section smoothly when its link is clicked.
export default class extends Controller {
  static targets = ["link"]

  connect() {
    this.onScroll = () => {
      if (this.pinned && this.settled) {
        this.pinned = null
        this.settled = false
      }
      if (this.frame) return
      this.frame = requestAnimationFrame(() => {
        this.frame = null
        this.highlight()
      })
    }

    window.addEventListener("scroll", this.onScroll, { passive: true })
    window.addEventListener("resize", this.onScroll, { passive: true })
    this.highlight()
  }

  disconnect() {
    window.removeEventListener("scroll", this.onScroll)
    window.removeEventListener("resize", this.onScroll)
    window.removeEventListener("scrollend", this.settle)
    clearTimeout(this.settleTimer)
    if (this.frame) cancelAnimationFrame(this.frame)
  }

  go(event) {
    const link = event.currentTarget
    const section = this.sectionOf(link)
    if (!section) return

    event.preventDefault()

    const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches
    section.scrollIntoView({ behavior: reduce ? "auto" : "smooth", block: "start" })
    history.replaceState(history.state, "", link.hash)

    // The last sections may never reach the top of the window, so the link
    // clicked stays current until the reader scrolls on after the jump.
    this.pin(link)
  }

  pin(link) {
    this.pinned = link
    this.settled = false
    this.setCurrent(link)

    clearTimeout(this.settleTimer)
    window.removeEventListener("scrollend", this.settle)
    window.addEventListener("scrollend", this.settle, { once: true })
    // scrollend never fires when the page is already in place or the
    // browser lacks it.
    this.settleTimer = setTimeout(this.settle, 1000)
  }

  // The jump is over. When the reader scrolled away from the section
  // during it, the mark follows the scroll again at once.
  settle = () => {
    clearTimeout(this.settleTimer)
    window.removeEventListener("scrollend", this.settle)
    this.settled = true

    const section = this.pinned && this.sectionOf(this.pinned)
    if (section && Math.abs(section.getBoundingClientRect().top) > 2 && !this.atBottom()) {
      this.pinned = null
      this.highlight()
    }
  }

  atBottom() {
    return window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 2
  }

  highlight() {
    if (this.pinned) return

    // A section is current once its top passes a third of the window; at the
    // bottom of the page the last section is current.
    const line = window.innerHeight / 3
    const atBottom = this.atBottom()
    let current = null

    for (const link of this.linkTargets) {
      const section = this.sectionOf(link)
      if (!section) continue
      if (atBottom || section.getBoundingClientRect().top <= line) current = link
    }

    this.setCurrent(current || this.linkTargets[0])
  }

  setCurrent(current) {
    for (const link of this.linkTargets) {
      if (link === current) {
        link.setAttribute("aria-current", "location")
      } else {
        link.removeAttribute("aria-current")
      }
    }
  }

  // Sections are looked up on every call: htmx swaps some of them in place.
  sectionOf(link) {
    return document.getElementById(decodeURIComponent(link.hash.slice(1)))
  }
}
