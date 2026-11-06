import { Controller } from "@hotwired/stimulus"

// The Audience part of the general settings: the subscribe checkbox is
// only available while the journal is open to anyone on the internet, and an
// author whose email subscribers would stop getting posts (subscriptions
// are live now and the form turns them off or narrows the journal) is warned
// before saving.
//
//   data-audience-public-value  the visibility value of "anyone on the internet"
//   data-audience-live-value    whether the stored settings let people subscribe
export default class extends Controller {
  static targets = ["select", "box", "explain", "warning"]
  static values = { public: String, live: Boolean }

  connect() {
    this.update()
  }

  update() {
    const open = this.selectTarget.value === this.publicValue

    this.boxTarget.disabled = !open
    if (!open) {
      this.boxTarget.checked = false
    }

    this.explainTarget.hidden = !open

    this.warningTargets.forEach((el) => {
      el.hidden = !(this.liveValue && !(open && this.boxTarget.checked))
    })
  }
}
