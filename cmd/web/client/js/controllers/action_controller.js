import { Controller } from "@hotwired/stimulus"
import { runAction, reloadPage, reportError } from "../lib"

export default class extends Controller {
  static values = {
    action: String,
    prompt: String,
    promptField: String,
    skipReload: Boolean,
  }

  connect() {
    const existingExt = this.element.getAttribute("hx-ext") || "";
    const newExt = existingExt ? existingExt + ",json-enc" : "json-enc";
    this.element.setAttribute("hx-ext", newExt);
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

    const onSuccess = this.skipReloadValue ? () => {} : reloadPage

    runAction(this.actionValue, this.element.dataset, extraFields, this.element)
    .then(onSuccess, reportError)
  }
}
