import { Controller } from "@hotwired/stimulus"

// The post form re-renders itself in place. Its submit buttons are rendered
// disabled so that a click before htmx has processed the swapped-in form
// cannot submit it natively (no CSRF header, 403). They are enabled once
// htmx is bound to the form.
export default class extends Controller {
  // notify is the "Email this post to subscribers" offer, which only a
  // public post can take up.
  static targets = ["notify"]

  connect() {
    window.htmx.process(this.element)

    this.element.querySelectorAll("button[data-postform-pending]").forEach((btn) => {
      btn.disabled = false
      btn.removeAttribute("data-postform-pending")
    })

    this.visibility()

    // A new post's text is kept in a scratch pad. When another tab or device
    // wrote the pad, the status shows a notice (data-scratch-conflict) and
    // this form stops autosaving: only a click (submit) or an explicit save
    // (postform:save) goes out until the author picks one of the notice's
    // choices, whose response replaces the notice.
    this.pauseAutosave = (event) => {
      const trigger = event.detail.requestConfig?.triggeringEvent?.type

      if (this.conflicted && trigger !== "submit" && trigger !== "postform:save") {
        event.preventDefault()
      }
    }

    this.element.addEventListener("htmx:beforeRequest", this.pauseAutosave)
  }

  disconnect() {
    this.element.removeEventListener("htmx:beforeRequest", this.pauseAutosave)
  }

  get conflicted() {
    return this.element.querySelector("[data-scratch-conflict]") !== null
  }

  // "Load the newer text": a fresh page load shows the pad as it is now. It
  // replaces the history entry, so that the browser doesn't bring the typed
  // text back into the form.
  loadNewer() {
    window.location.replace(window.location.href)
  }

  // Shows the subscriber offer while Public is selected.
  visibility() {
    const visibility = this.element.querySelector("input[name=visibility]:checked")?.value

    this.notifyTargets.forEach((el) => {
      el.hidden = visibility !== "public"
    })
  }

  // The preview renders the saved post, and the form saves only two seconds
  // after an edit, so a preview opened right after an edit would show the
  // post without it. Save now and open the preview once the save is back.
  preview(event) {
    // the other tab's text would open, not this one's: show the notice
    if (this.conflicted) {
      event.preventDefault()
      this.element.querySelector("[data-scratch-conflict]").scrollIntoView({ block: "center" })

      return
    }

    // a new post that is still empty has no pad to open
    if (!this.element.querySelector("input[name=post_id]") &&
        ["subject", "url", "body"].every((name) => this.element.elements[name].value === "")) {
      event.preventDefault()

      return
    }

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

      // this very save found the pad changed elsewhere: the notice says so
      if (this.conflicted) {
        win?.close()

        return
      }

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
