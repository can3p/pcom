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

  // The preview renders the saved post, and the form saves only two seconds
  // after an edit, so a preview opened right after an edit would show the
  // post without it. Save now and open the preview once the save is back.
  preview(event) {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
      return
    }

    event.preventDefault()

    const href = event.currentTarget.href
    // a window opened when the save is back no longer counts as opened by
    // the click and is blocked as a popup, so open it now and point it at
    // the preview later
    const win = window.open("", "_blank")
    const save = new CustomEvent("postform:save")

    // the form can be swapped out by the response (a validation error), in
    // which case htmx reports the request on its parent, so listen on the
    // document and recognize the request by its triggering event
    const done = (ev) => {
      if (ev.detail.requestConfig?.triggeringEvent !== save) {
        return
      }

      document.removeEventListener("htmx:afterRequest", done)

      if (win) {
        win.location = href
      } else {
        window.open(href, "_blank")
      }
    }

    document.addEventListener("htmx:afterRequest", done)
    this.element.dispatchEvent(save)
  }
}
