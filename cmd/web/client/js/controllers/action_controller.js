import { Controller } from "@hotwired/stimulus"
import { runAction, reloadPage, reportError } from "../lib"

export default class extends Controller {
  static values = {
    action: String,
    prompt: String,
    promptField: String,
  }

  run(event) {
    event.preventDefault()

    let extraFields = null

    if (!!this.promptValue) {
      if (this.promptFieldValue !=="") {
        const promptResult = prompt(this.promptValue)

        if (promptResult === null) {
          return
        }

        extraFields = {
          [this.promptFieldValue]: promptResult,
        }
      } else if (!confirm(this.promptValue)) {
        return
      }
    }

    runAction(this.actionValue, this.element.dataset, extraFields)
    .then(reloadPage, reportError)
  }
}
