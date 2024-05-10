

export async function runAction(name, dataset, extraFields) {
  const url = `/controls/action/${name}`
  let payload = toPayload(dataset)

  if (!!extraFields) {
    payload = { ...payload, ...extraFields }
  }

  return fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
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
