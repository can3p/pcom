import { Controller } from "@hotwired/stimulus"

export default class extends Controller {
  static values = { }

  connect() {
    this.target = this.element.querySelector(".block-container-spoiler-summary")
    this.content = this.element.querySelector(".block-container-spoiler-content")

    this.target.addEventListener("click", (e) => {
      e.preventDefault()
      this.show()
    }, false)
  }

  show() {
    this.content.classList.add("show")
    this.target.classList.add("d-none")
  }
}
