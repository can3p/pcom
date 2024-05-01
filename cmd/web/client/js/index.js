import htmx from 'htmx.org/dist/htmx.js';
import hyperscript from 'hyperscript.org';
import { Application } from "@hotwired/stimulus"
import { definitionsFromContext } from "@hotwired/stimulus-webpack-helpers"

window.htmx = htmx
window._hyperscript = hyperscript
window._hyperscript.browserInit()

// this is kinda lame since that means that htmx will never execute any new js
// however that's the closest we can get to turbo behavior which is smart enough
// to only load scripts it has not seen before or reload the page if the same
// script changes
htmx.config.allowScriptTags = false;

window.Stimulus = Application.start()
const context = require.context("./controllers", true, /\.js$/)
Stimulus.load(definitionsFromContext(context))
