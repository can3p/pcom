import { Controller } from "@hotwired/stimulus"

export default class extends Controller {
  connect() {
    this.target = document.querySelector(this.targetValue)
    let targetEl = document.querySelector(`${this.targetValue} [role=close]`)

    this.element.addEventListener("change", (e) => {
      if (this.element.value != "") {
        this.element.form.submit()
      }
    }, false)
  }
}
