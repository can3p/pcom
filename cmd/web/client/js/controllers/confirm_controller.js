import { Controller } from "@hotwired/stimulus"

export default class extends Controller {
  static values = {
    message: String
  }

  connect() {
    this.element.addEventListener("click", (e) => {
      if (!window.confirm(this.messageValue)) {
        e.preventDefault()
        e.stopPropagation()
      }
    }, false)
  }
}
