

export async function runAction(name, dataset) {
  const url = `/controls/action/${name}`
  console.log("runAction", url)
  const payload = toPayload(dataset)

  return fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(payload),
  })
    .then((response) => response.json())
    .then((data) => {
      console.log("data", data)
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
