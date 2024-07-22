import { Controller } from "@hotwired/stimulus"

function generateToast(msg) {
  let html = `<div data-controller="toast" class="toast text-bg-danger align-items-center border-0 fade show" role="alert" aria-live="assertive" aria-atomic="true">
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

    this.handler = (e) => {
      this.element.appendChild(generateToast((e.detail.explanation)))
    }

    htmx.on("operation:error", this.handler)
  }

  disconnect() {
    htmx.off("operation:error", this.handler)
  }
}
