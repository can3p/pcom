import { Controller } from "@hotwired/stimulus"

export default class extends Controller {
  static values = {
    target: String,
    hideTarget: Boolean
  }

  connect() {
    this.target = document.querySelector(this.targetValue)
    let targetEl = document.querySelector(`${this.targetValue} [role=close]`)

    this.element.addEventListener("click", (e) => {
      e.preventDefault()
      this.toggle()
    }, false)
    targetEl.addEventListener("click", () => this.hide(), false)
  }

  show() {
    this.target.classList.add("show")

    if (this.hideTargetValue) {
      this.element.classList.add("d-none")
    }
  }

  hide() {
    this.target.classList.remove("show")

    if (this.hideTargetValue) {
      this.element.classList.remove("d-none")
    }
  }

  toggle() {
    if (this.target.classList.contains("show")) {
      this.hide()
      return
    }

    this.show()
  }
}
