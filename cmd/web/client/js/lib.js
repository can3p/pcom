

export async function runAction(name, dataset, extraFields, element) {
  const url = `/controls/action/${name}`
  let payload = toPayload(dataset)

  if (!!extraFields) {
    payload = { ...payload, ...extraFields }
  }

  let headers = {}

  let addHeadersStr = document.body.getAttribute("hx-headers")

  if (addHeadersStr) {
    try {
      let parsed = JSON.parse(addHeadersStr)
      headers = { ...headers, ...parsed}
    } catch(e) {
      console.warn("failed to parse additional headers", e)
    }
  }

  return new Promise((resolve, reject) => {
    htmx.ajax('POST', url, {
      source: element,
      target: element.getAttribute('hx-target') || element,
      swap: element.getAttribute('hx-swap') || 'none',
      headers: headers,
      values: payload,
      ext: 'json-enc',
    }).then(() => {
      resolve({})
    }).catch((err) => {
      reject(err?.xhr?.responseText || 'Unknown error')
    })
  })
}

export function reloadPage() {
  window.location.reload()
}

export function reportError(err) {
  let payload = err

  if (!payload.explanation) {
    payload = {
      explanation: payload
    }
  }

  htmx.trigger('body', "operation:error", payload)
}

function toPayload(dataset) {
  let out = {}

  for (let k in dataset) {
    if (k === "action" || k === "controller" || k.match(/Value$/)) {
      continue
    }

    out[k] = dataset[k]
  }

  return out
}
