// The capture page's own bundle, loaded instead of main.js so the page opens
// fast on a weak network: Stimulus, the capture controller and the menu
// toggle of the header. No htmx: the page's links load whole pages.
import { Application } from "@hotwired/stimulus"
import NotesCaptureController from "./capture/notes_capture_controller.js"
import ToggleController from "./controllers/toggle_controller.js"

const app = Application.start()
app.register("notes-capture", NotesCaptureController)
app.register("toggle", ToggleController)
