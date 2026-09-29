import { Controller } from "@hotwired/stimulus"

// The post form re-renders itself in place. Its submit buttons are rendered
// disabled so that a click before htmx has processed the swapped-in form
// cannot submit it natively (no CSRF header, 403). They are enabled once
// htmx is bound to the form.
export default class extends Controller {
  connect() {
    window.htmx.process(this.element)

    this.element.querySelectorAll("button[data-postform-pending]").forEach((btn) => {
      btn.disabled = false
      btn.removeAttribute("data-postform-pending")
    })
  }
}
