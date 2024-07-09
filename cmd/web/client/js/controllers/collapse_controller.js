import Collapse from 'bootstrap/js/dist/collapse';
import { Controller } from "@hotwired/stimulus"

export default class extends Controller {
  connect() {
    new Collapse(this.element, { toggle: false })
  }
}
