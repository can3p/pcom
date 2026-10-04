import { Controller } from "@hotwired/stimulus"

export default class extends Controller {
  static values = {
    target: String,
    focus: String,
    hideTarget: Boolean,
    closeOthersSelector: String
  }

  connect() {
    this.target = document.querySelector(this.targetValue)
    let targetEl = document.querySelector(`${this.targetValue} [role=close]`)

    this.element.addEventListener("click", (e) => {
      e.preventDefault()
      this.toggle()
    }, false)
    if (targetEl) {
      targetEl.addEventListener("click", () => this.hide(), false)
    }
  }

  show() {
    // Close other toggles if selector is provided
    if (this.closeOthersSelectorValue && this.closeOthersSelectorValue.trim() !== "") {
      this.closeOtherToggles()
    }

    this.target.classList.add("show")

    if (this.focusValue) {
      document.querySelector(this.focusValue).focus()
    }

    if (this.hideTargetValue) {
      this.element.hidden = true
    }

    if (this.element.hasAttribute("aria-expanded")) {
      this.element.setAttribute("aria-expanded", "true")
    }
  }

  closeOtherToggles() {
    // Find all elements matching the selector
    const otherToggles = document.querySelectorAll(this.closeOthersSelectorValue)

    otherToggles.forEach(toggle => {
      // Skip the current target
      if (toggle !== this.target && toggle.classList.contains("show")) {
        toggle.classList.remove("show")

        // Also show any hidden toggle buttons
        const toggleButton = document.querySelector(`[data-toggle-target-value="#${toggle.id}"]`)
        if (toggleButton && toggleButton.hidden) {
          toggleButton.hidden = false
        }
      }
    })
  }

  hide() {
    this.target.classList.remove("show")

    if (this.hideTargetValue) {
      this.element.hidden = false
    }

    if (this.element.hasAttribute("aria-expanded")) {
      this.element.setAttribute("aria-expanded", "false")
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
