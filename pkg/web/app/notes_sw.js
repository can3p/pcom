// The notes service worker, served from /controls/notes/sw.js, so its scope is
// the notes pages and nothing else.
//
// It uploads the captures the capture page (/controls/notes/new) writes to
// IndexedDB, oldest first, retrying while the network or the server fails,
// and tells the open pages when one is saved. It also keeps the capture page
// and the assets that page names, so the page opens without a network; every
// other request goes to the network as it would without it.
//
// It is also the share target's first stop (site.webmanifest): what another
// app shares is queued like a capture made on the page, so a share works
// offline too, and the phone is sent on to the capture page, which shows it
// as the last capture. Only a share made while no worker controls the page
// reaches the server's route.
//
// The record format is shared with cmd/web/client/js/notes/capture.js.

const DB_NAME = "pcom-notes"
const DB_VERSION = 1
const STORE = "captures"
const CACHE = "pcom-notes-capture-1"
const PAGE = "/controls/notes/new"
const UPLOAD = "/controls/action/note_capture"
const DELETE = "/controls/action/note_delete"
const SHARE = "/controls/notes/share"
const SYNC_TAG = "notes-upload"
// waits between attempts double from the first to the last, which repeats
// until the attempts run out; a message from a page or a sync starts over
const FIRST_WAIT = 1000
const LAST_WAIT = 30000
const ATTEMPTS = 12

self.addEventListener("install", () => self.skipWaiting())

self.addEventListener("activate", (event) => {
  event.waitUntil((async () => {
    for (const key of await caches.keys()) {
      if (key !== CACHE) await caches.delete(key)
    }

    await self.clients.claim()
    await flush()
  })())
})

self.addEventListener("message", (event) => {
  const msg = event.data || {}

  if (msg.type === "keep") event.waitUntil(keep(msg.urls || []))
  if (msg.type === "flush") event.waitUntil(flush())
})

self.addEventListener("sync", (event) => {
  if (event.tag !== SYNC_TAG) return

  event.waitUntil(flush().then(async () => {
    // a rejection makes the browser try the sync again later
    if ((await records()).some((r) => r.status === "queued")) throw new Error("captures still queued")
  }))
})

self.addEventListener("fetch", (event) => {
  const req = event.request

  if (req.method === "POST" && new URL(req.url).pathname === SHARE) {
    event.respondWith(share(req))
    return
  }

  if (req.method !== "GET") return

  if (req.mode === "navigate") {
    if (new URL(req.url).pathname === PAGE) event.respondWith(page(req))

    return
  }

  if (["script", "style", "font"].includes(req.destination)) {
    event.respondWith(caches.match(req, { cacheName: CACHE }).then((hit) => hit || fetch(req)))
  }
})

// page answers the capture page from the network, keeping the answer, and
// from the cache when the network fails.
async function page(req) {
  try {
    const resp = await fetch(req)

    if (resp.ok && !resp.redirected) {
      const copy = resp.clone()
      const cache = await caches.open(CACHE)
      await cache.put(PAGE, copy)
    }

    return resp
  } catch (err) {
    const hit = await caches.match(PAGE, { cacheName: CACHE })
    if (hit) return hit

    throw err
  }
}

// share queues what the share sheet sent and sends the phone to the capture
// page. The record has no CSRF token yet: the page gives it the session's
// when it opens, and asks for the upload. When the share can't be kept, the
// server's route takes it.
async function share(req) {
  const fallback = req.clone()

  try {
    const form = await req.formData()
    const files = (name) => form.getAll(name).filter((f) => f instanceof Blob && f.size > 0)
    const text = shareText(form.get("title"), form.get("text"), form.get("url"))
    const images = files("image")
    const audio = files("audio")[0] || null

    if (text || images.length || audio) {
      await withStore("readwrite", (s) => s.put({
        id: crypto.randomUUID(),
        savedAt: Date.now(),
        capturedAt: new Date().toISOString(),
        csrf: "",
        status: "queued",
        kind: "share",
        text,
        images,
        audio,
        seconds: 0,
        photos: images.length,
        voice: !!audio,
      }))
    }
  } catch (_) {
    return fetch(fallback)
  }

  return Response.redirect(PAGE, 303)
}

// shareText joins the title, text and link of a share, one per line, leaving
// out a part another one already holds; a link alone stays a bare link, and
// the CRLF line breaks of the form become LF. It matches shareText in
// pkg/web/app/routes_notes_share.go.
function shareText(...values) {
  const parts = values.map((v) => (typeof v === "string" ? v.replaceAll("\r\n", "\n").trim() : ""))

  return parts
    .filter((p, i) => p && !parts.some((o, j) => j !== i && o.includes(p) && (o !== p || j < i)))
    .join("\n")
}

// keep stores the capture page and the assets it loaded.
async function keep(urls) {
  const cache = await caches.open(CACHE)

  for (const url of [PAGE, ...urls]) {
    try {
      const resp = await fetch(url, { credentials: "same-origin" })
      if (resp.ok && !resp.redirected) await cache.put(url, resp)
    } catch (_) {
      // offline or refused: kept on the next visit
    }
  }
}

function openDB() {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION)
    req.onupgradeneeded = () => req.result.createObjectStore(STORE, { keyPath: "id" })
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}

// withStore runs fn on the store in one transaction and returns the result of
// the request fn returns, once the transaction is done.
async function withStore(mode, fn) {
  const db = await openDB()

  try {
    return await new Promise((resolve, reject) => {
      const t = db.transaction(STORE, mode)
      const req = fn(t.objectStore(STORE))
      t.oncomplete = () => resolve(req ? req.result : undefined)
      t.onerror = () => reject(t.error)
      t.onabort = () => reject(t.error)
    })
  } finally {
    db.close()
  }
}

function records() {
  return withStore("readonly", (s) => s.getAll())
}

// settle applies change to the record with id, if the page hasn't deleted it,
// and says whether it was still there.
async function settle(id, change) {
  let found = false

  await withStore("readwrite", (s) => {
    const get = s.get(id)
    get.onsuccess = () => {
      if (!get.result) return

      found = true
      change(get.result)
      s.put(get.result)
    }

    return null
  })

  return found
}

async function notify() {
  for (const client of await self.clients.matchAll({ type: "window", includeUncontrolled: true })) {
    client.postMessage({ type: "captures" })
  }
}

let running = null
let again = false
let wake = null

// flush uploads every queued capture; a call while one runs makes it start
// over at once instead of waiting out its pause.
function flush() {
  if (running) {
    again = true
    if (wake) wake()

    return running
  }

  running = (async () => {
    try {
      let wait = FIRST_WAIT

      for (let attempt = 0; attempt < ATTEMPTS; attempt++) {
        again = false

        const done = await uploadAll()
        if (done && !again) return
        if (!again) await pause(wait)

        wait = again ? FIRST_WAIT : Math.min(wait * 2, LAST_WAIT)
      }
    } finally {
      running = null
    }
  })()

  return running
}

function pause(ms) {
  return new Promise((resolve) => {
    const done = () => {
      clearTimeout(timer)
      wake = null
      resolve()
    }
    const timer = setTimeout(done, ms)
    wake = done
  })
}

// uploadAll sends the queued captures, oldest first, and says whether none
// is left to retry. A share waits for the page to give it a CSRF token.
async function uploadAll() {
  const queued = (await records())
    .filter((r) => r.status === "queued" && r.csrf)
    .sort((a, b) => a.savedAt - b.savedAt)

  for (const rec of queued) {
    if (!(await upload(rec))) return false
  }

  return true
}

// upload sends one capture and says whether it is settled: saved, or refused
// for good by the server. A network failure, a server error or an expired
// login (a redirect) is retried.
async function upload(rec) {
  const form = new FormData()
  form.append("kind", rec.kind || "note")
  form.append("captured_at", rec.capturedAt)
  if (rec.text) form.append("text", rec.text)

  rec.images.forEach((img, i) => form.append("image", img, `photo-${i + 1}.jpg`))

  if (rec.audio) form.append("audio", rec.audio, "recording." + (rec.audio.type.includes("mp4") ? "m4a" : "webm"))

  let resp

  try {
    resp = await fetch(UPLOAD, {
      method: "POST",
      body: form,
      headers: { "X-CSRFToken": rec.csrf },
      credentials: "same-origin",
      redirect: "manual",
    })
  } catch (_) {
    return false
  }

  if (resp.ok) {
    const out = await resp.json()
    const kept = await settle(rec.id, (r) => {
      r.status = "uploaded"
      r.noteId = out.note_id
      r.deletePrompt = out.delete_prompt
      r.images = []
      r.audio = null
    })

    // deleted on the page while it was on its way
    if (!kept) await discard(rec, out.note_id)

    await notify()

    return true
  }

  if (resp.status === 400) {
    let explanation = "The capture was refused."

    try {
      explanation = (await resp.json()).explanation || explanation
    } catch (_) {
      // not the action's JSON
    }

    await settle(rec.id, (r) => {
      r.status = "failed"
      r.error = explanation
    })
    await notify()

    return true
  }

  return false
}

async function discard(rec, noteId) {
  try {
    await fetch(DELETE, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRFToken": rec.csrf },
      body: JSON.stringify({ noteId }),
      credentials: "same-origin",
      redirect: "manual",
    })
  } catch (_) {
    // the note stays; the author deletes it from the list
  }
}
