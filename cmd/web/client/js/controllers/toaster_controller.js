import { Controller } from "@hotwired/stimulus"

function generateToast(kind, msg) {
  const tmpl = document.createElement('template');
  tmpl.innerHTML = `<div data-controller="toast" class="toast ${kind}" role="alert">
  <div class="toast-body"></div>
  <button type="button" class="toast-close" data-toast-target="close" aria-label="Close"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M6 6l12 12"></path><path d="M18 6L6 18"></path></svg></button>
</div>`
  // the message is text, not markup
  tmpl.content.querySelector(".toast-body").textContent = msg
  return tmpl.content;
}


export default class extends Controller {
  static values = {
    upload: String,
  }

  connect() {

    let handler = (type) => {
      return (e) => {
        this.element.appendChild(generateToast(type, e.detail.explanation))
      }
    }

    this.success = handler("ok")
    this.error = handler("err")

    htmx.on("operation:success", this.success)
    htmx.on("operation:error", this.error)
  }

  disconnect() {
    htmx.off("operation:success", this.success)
    htmx.off("operation:error", this.error)
  }
}
