import htmx from 'htmx.org/dist/htmx.js';
import hyperscript from 'hyperscript.org';
import { Application } from "@hotwired/stimulus"
import { definitionsFromContext } from "@hotwired/stimulus-webpack-helpers"
import 'lazysizes';

// we really want to load inline styles for youtube embed
window.liteYouTubeNonce = document.body.dataset.styleNonce

// we load the lib asyncronously since there is no other quick way
// to set the global variable above
import('@justinribeiro/lite-youtube')

// need to do this to please content security policy
// https://github.com/bigskysoftware/htmx/issues/862
htmx.config.includeIndicatorStyles = false

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
