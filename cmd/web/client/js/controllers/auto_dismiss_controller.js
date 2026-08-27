import { Controller } from "@hotwired/stimulus"

export default class extends Controller {
  connect() {
    this.dismissTimer = setTimeout(() => {
      this.element.style.transition = "opacity 500ms"
      this.element.style.opacity = "0"
      this.removeTimer = setTimeout(() => this.element.remove(), 500)
    }, 3_000)
  }

  disconnect() {
    clearTimeout(this.dismissTimer)
    clearTimeout(this.removeTimer)
  }
}
