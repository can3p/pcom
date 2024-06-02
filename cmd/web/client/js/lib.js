

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


  return fetch(url, {
    method: 'POST',
    headers: headers,
    body: JSON.stringify(payload),
  })
    .then((response) => response.json())
    .then((data) => {
      if (data.explanation) {
        return Promise.reject(data.explanation)
      }
    })
}

export function reloadPage() {
  window.location.reload()
}

export function reportError(err) {
  alert(err)
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
