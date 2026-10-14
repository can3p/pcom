import { Controller } from "@hotwired/stimulus"

// The post editor's grammar check and style review. This stub is all that
// every page downloads; the panel itself (../proofread/panel.js) is a separate
// chunk fetched when the editor appears.
export default class extends Controller {
  static targets = ["panel", "limits", "grammarButton", "styleButton"]
  static values = { grammarUrl: String, styleUrl: String, noteUrl: String, resetUrl: String, stateUrl: String }

  async connect() {
    this.gone = false

    const { Panel } = await import("../proofread/panel.js")

    if (!this.gone) this.impl = new Panel(this)
  }

  disconnect() {
    this.gone = true
    this.impl?.disconnect()
    this.impl = null
  }

  checkGrammar() {
    this.impl?.checkGrammar()
  }

  reviewStyle() {
    this.impl?.reviewStyle()
  }
}
