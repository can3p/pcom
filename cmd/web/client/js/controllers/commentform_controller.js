import { Controller } from "@hotwired/stimulus"

export default class extends Controller {
  connect() {
    this.handler = (e) => {
      if (e.keyCode == 13 && (e.ctrlKey || e.metaKey)) {
        htmx.trigger(this.element, "submit")
      }
    }

    this.element.addEventListener("keydown", this.handler)
  }

  disconnect() {
    this.element.removeEventListener("keydown", this.handler)
  }
}
