

export async function runAction(name, dataset, extraFields) {
  const url = `/controls/action/${name}`
  let payload = toPayload(dataset)

  if (!!extraFields) {
    payload = { ...payload, ...extraFields }
  }

  let headers = {
    'Content-Type': 'application/json',
  }


  let addHeadersStr = document.body.getAttribute("hx-headers")

  if (addHeadersStr) {
    try {
      let parsed = JSON.parse(addHeadersStr)
      headers = { ...headers, ...parsed}
    } catch(e) {
      console.warn("failed to parse additional headers", e)
    }
  }

  let response = await fetch(url, {
    method: 'POST',
    headers: headers,
    body: JSON.stringify(payload),
  })

  if (!response.ok) {
    let respBody = await response.text()

    if (respBody != "") {
      try {
        respBody = JSON.parse(respBody)
      } catch(e){}
    } else {
      respBody = `Unknown error: Failed with response code ${response.status}`
    }

    throw respBody;
  }

  let j = await response.json()
  return j
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
