import { Controller } from "@hotwired/stimulus"
import { runAction, reloadPage, reportError } from "../lib"

export default class extends Controller {
  static values = {
    action: String,
    prompt: String,
    streamId: String,
  }

  run(event) {
    event.preventDefault()

    if (!!this.promptValue && !confirm(this.promptValue)) {
      return
    }

    runAction(this.actionValue, this.element.dataset)
    .then(reloadPage, reportError)
  }
}
