import { Controller } from "@hotwired/stimulus"

export default class extends Controller {
  connect() {
    this.element.querySelector("[data-bs-dismiss]").addEventListener('click', (e) => {
      this.element.parentNode.removeChild(this.element)
    }, false)

    this.timer = setTimeout(() => {
      this.element.parentNode.removeChild(this.element)
    }, 10_000)
  }

  disconnect() {
    clearTimeout(this.timer)
  }
}
