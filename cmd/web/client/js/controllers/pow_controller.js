import { Controller } from "@hotwired/stimulus"

// Solves the proof-of-work challenge of a form (pkg/abuse) in the reader's
// browser, so a script that posts the form pays for every submission.
//
//   <form data-controller="pow" data-pow-challenge-value="{{ .Token }}">
//     <input type="hidden" name="pow_nonce" data-pow-target="nonce">
//
// The challenge is `<issued>.<salt>.<difficulty>.<signature>`. The answer is
// the decimal nonce for which SHA-256 over the UTF-8 bytes of the challenge
// followed by the nonce's digits starts with <difficulty> zero bits: the
// same bytes pkg/abuse hashes. The work starts when the reader first
// touches the form and runs in batches that yield to the event loop; a
// submit that arrives before the answer is held (button text "Checking…")
// and sent with requestSubmit, so htmx and validation see an ordinary submit.
const BATCH = 4096
const WAITING = "Checking…"

export default class extends Controller {
  static values = { challenge: String }
  static targets = ["nonce"]

  connect() {
    this.solving = null
    this.nonce = null
    this.onSubmit = (e) => this.submit(e)
    this.onTouch = () => this.start()
    this.element.addEventListener("submit", this.onSubmit, true)
    this.element.addEventListener("focusin", this.onTouch, { once: true })
    this.element.addEventListener("pointerdown", this.onTouch, { once: true })
  }

  disconnect() {
    this.element.removeEventListener("submit", this.onSubmit, true)
    this.element.removeEventListener("focusin", this.onTouch)
    this.element.removeEventListener("pointerdown", this.onTouch)
    this.stopped = true
  }

  start() {
    if (!this.solving) {
      this.solving = this.solve().then((nonce) => {
        this.nonce = nonce
        this.nonceTarget.value = nonce
        return nonce
      })
    }
    return this.solving
  }

  async submit(e) {
    if (this.nonce !== null || !window.crypto?.subtle) return

    e.preventDefault()
    e.stopImmediatePropagation()

    const button = e.submitter || this.element.querySelector("[type=submit]")
    const label = button && (button.tagName === "INPUT" ? button.value : button.textContent)
    if (button) {
      this.setLabel(button, WAITING)
      button.disabled = true
    }

    await this.start()

    if (this.stopped) return
    if (button) {
      this.setLabel(button, label)
      button.disabled = false
    }
    this.element.requestSubmit(e.submitter || undefined)
  }

  setLabel(button, text) {
    if (button.tagName === "INPUT") button.value = text
    else button.textContent = text
  }

  async solve() {
    const challenge = this.challengeValue
    const bits = parseInt(challenge.split(".")[2], 10)
    const prefix = new TextEncoder().encode(challenge)
    const hash = async (n) => {
      const digits = new TextEncoder().encode(String(n))
      const data = new Uint8Array(prefix.length + digits.length)
      data.set(prefix)
      data.set(digits, prefix.length)
      return new Uint8Array(await crypto.subtle.digest("SHA-256", data))
    }

    for (let from = 0; !this.stopped; from += BATCH) {
      const sums = await Promise.all(Array.from({ length: BATCH }, (_, i) => hash(from + i)))
      for (let i = 0; i < BATCH; i++) {
        if (leadingZeroBits(sums[i]) >= bits) return String(from + i)
      }
      // Let the page paint and respond between batches.
      await new Promise((resolve) => setTimeout(resolve))
    }
    return null
  }
}

function leadingZeroBits(sum) {
  let n = 0
  for (const b of sum) {
    if (b === 0) {
      n += 8
      continue
    }
    return n + Math.clz32(b) - 24
  }
  return n
}
