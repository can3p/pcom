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

    navigator.clipboard.writeText(this.copyValue)

    this.element.classList.remove("bi-clipboard")
    this.element.classList.add("bi-check2")

    setTimeout(() => {
      this.element.classList.remove("bi-check2")
      this.element.classList.add("bi-clipboard")
    }, 300)
  }
}
