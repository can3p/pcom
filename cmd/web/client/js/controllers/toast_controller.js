import { Controller } from "@hotwired/stimulus"

export default class extends Controller {
  static targets = ["close"]

  connect() {
    this.closeTarget.addEventListener('click', () => this.element.remove(), false)

    this.timer = setTimeout(() => this.element.remove(), 10_000)
  }

  disconnect() {
    clearTimeout(this.timer)
  }
}
