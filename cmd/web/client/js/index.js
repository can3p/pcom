import htmx from 'htmx.org';
import 'htmx-ext-head-support';
import hyperscript from 'hyperscript.org';
import { Application } from "@hotwired/stimulus"
import { definitionsFromContext } from "@hotwired/stimulus-webpack-helpers"
import 'lazysizes';
import '@justinribeiro/lite-youtube'

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

// Define JSON encoding extension for htmx
htmx.defineExtension('json-enc', {
    onEvent: function (name, evt) {
        if (name === "htmx:configRequest") {
            evt.detail.headers['Content-Type'] = "application/json";
        }
    },
    
    encodeParameters : function(xhr, parameters, elt) {
        xhr.overrideMimeType('text/json');
        return (JSON.stringify(parameters));
    }
});

window.Stimulus = Application.start()
const context = require.context("./controllers", true, /\.js$/)
Stimulus.load(definitionsFromContext(context))

// Handle htmx errors during page transitions
htmx.on('htmx:responseError', function(evt) {
  const statusCode = evt.detail.xhr.status;
  let message = `Failed to load page (Error ${statusCode})`;

  if (statusCode === 0) {
    message = 'Network error - please check your connection';
  } else if (statusCode >= 500) {
    message = 'Server error - please try again later';
  } else if (statusCode === 404) {
    message = 'Page not found';
  } else if (statusCode === 403) {
    message = 'Access denied';
  }

  htmx.trigger(document.body, 'operation:error', { explanation: message });
});

htmx.on('htmx:sendError', function(evt) {
  htmx.trigger(document.body, 'operation:error', {
    explanation: 'Network error - could not reach server'
  });
});
