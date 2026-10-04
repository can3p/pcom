import { Controller } from "@hotwired/stimulus"

export default class extends Controller {
  static values = {
    copy: String,
  }

  connect() {
    this.timer = null;

    this.element.addEventListener("click", (e) => {
      e.preventDefault()
      this.copy()
    }, false)
  }

  copy() {
    clearTimeout(this.timer)

    // a path ("/shared/...") is copied as the full address; any other value
    // (an API key) is copied as it is
    const value = this.copyValue.startsWith("/")
      ? new URL(this.copyValue, window.location.origin).href
      : this.copyValue

    navigator.clipboard.writeText(value)

    // the control says "Copied" for a moment
    if (this.original === undefined && this.element.textContent.trim() !== "") {
      this.original = this.element.textContent
      this.element.textContent = "Copied"
    }

    this.timer = setTimeout(() => {
      if (this.original !== undefined) {
        this.element.textContent = this.original
        this.original = undefined
      }
    }, 1500)
  }
}
