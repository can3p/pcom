import { Controller } from "@hotwired/stimulus"

function generateToast(type, msg) {
  let html = `<div data-controller="toast" class="toast text-bg-${type} align-items-center border-0 fade show" role="alert" aria-live="assertive" aria-atomic="true">
  <div class="d-flex">
    <div class="toast-body">${msg}</div>
    <button type="button" class="btn-close btn-close-white me-2 m-auto" data-bs-dismiss="toast" aria-label="Close"></button>
  </div>
</div>`

  var tmpl = document.createElement('template');
  tmpl.innerHTML = html;
  return tmpl.content;
}


export default class extends Controller {
  static values = {
    upload: String,
  }

  connect() {

    let handler = (type) => {
      return (e) => {
        console.log("handler", type, e.detail)
        this.element.appendChild(generateToast(type, e.detail.explanation))
      }
    }

    this.success = handler("success")
    this.error = handler("danger")

    htmx.on("operation:success", this.success)
    htmx.on("operation:error", this.error)
  }

  disconnect() {
    htmx.off("operation:success", this.success)
    htmx.off("operation:error", this.error)
  }
}
