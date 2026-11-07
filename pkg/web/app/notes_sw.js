// The notes service worker, served from /controls/notes/sw.js, so its scope is
// the notes pages and nothing else.
//
// It uploads the captures the capture page (/controls/notes/new) writes to
// IndexedDB, oldest first, retrying while the network or the server fails,
// and tells the open pages when one is saved. It also keeps the capture page
// and the assets that page names, so the page opens without a network; every
// other request goes to the network as it would without it.
//
// It is also the share target's first stop (site.webmanifest). A share goes
// to the server's route as it is; only when the network fails is it queued
// like a capture made on the page, so a share works offline too, and the
// phone is sent on to the capture page, which shows it as the last capture.
// Queuing only on a network failure is what keeps a forged share out: a
// cross-site POST reaches the server without the session cookie (SameSite=Lax)
// and is sent to the login page, and the worker never keeps it to upload it
// later under the session's token.
//
// The registration URL carries the caps of what a queued share may hold, the
// same as the server's: audio_max for the recording, image_max for each photo
// and capture_max for all of it.
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
// the assets the capture page loads live under this path of their origin
const STATIC = "/static/"
const caps = new URL(self.location.href).searchParams
// a cap the URL leaves out keeps no share: 0 refuses any file and any text
const AUDIO_MAX = Number(caps.get("audio_max")) || 0
const IMAGE_MAX = Number(caps.get("image_max")) || 0
const CAPTURE_MAX = Number(caps.get("capture_max")) || 0
// waits between attempts double from the first to the last, which repeats
// until the attempts run out; a message from a page or a sync starts over
const FIRST_WAIT = 1000
const LAST_WAIT = 30000
const ATTEMPTS = 12
// a refused share keeps this much of its text, for the page's card
const EXCERPT_LENGTH = 80
const KIB = 1024
const MIB = 1024 * KIB

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

// share sends what the share sheet sent to the server's route, whose answer
// (the capture page, or the login page for a request without a session) the
// phone follows. Only when the network fails is the share queued.
async function share(req) {
  const offline = req.clone()

  try {
    return await fetch(req)
  } catch (_) {
    return queueShare(offline)
  }
}

// queueShare keeps a share made offline and sends the phone to the capture
// page. The record has no CSRF token yet: the page gives it the session's
// when it opens, and asks for the upload. A share over the caps is not kept:
// its record holds no files, only why, which the page shows.
async function queueShare(req) {
  const rec = {
    id: crypto.randomUUID(),
    savedAt: Date.now(),
    capturedAt: new Date().toISOString(),
    csrf: "",
    status: "queued",
    kind: "share",
    text: "",
    images: [],
    audio: null,
    seconds: 0,
    photos: 0,
    voice: false,
  }

  try {
    const form = await req.formData()
    const files = (name) => form.getAll(name).filter((f) => f instanceof Blob && f.size > 0)
    const text = shareText(form.get("title"), form.get("text"), form.get("url"))
    const images = files("image")
    const audio = files("audio")

    if (!text && !images.length && !audio.length) return Response.redirect(PAGE, 303)

    rec.photos = images.length
    rec.voice = audio.length > 0

    const refused = overCaps(text, images, audio)
    if (refused) {
      rec.status = "failed"
      rec.error = refused
      rec.text = text.slice(0, EXCERPT_LENGTH)
    } else {
      rec.text = text
      rec.images = images
      rec.audio = audio[0] || null
    }
  } catch (_) {
    rec.status = "failed"
    rec.error = "The share can't be read."
  }

  try {
    await withStore("readwrite", (s) => s.put(rec))
  } catch (_) {
    // no IndexedDB: the page says it can't keep notes
  }

  return Response.redirect(PAGE, 303)
}

// overCaps says why a share can't be kept offline, or "" when it can: the
// server's caps, as the registration URL gives them.
function overCaps(text, images, audio) {
  if (audio.length > 1) return "Send at most one recording."
  if (audio.some((a) => a.size > AUDIO_MAX)) return `The recording is larger than ${bytes(AUDIO_MAX)}.`
  if (images.some((i) => i.size > IMAGE_MAX)) return `A photo can be at most ${bytes(IMAGE_MAX)}.`

  const total = [...images, ...audio].reduce((sum, f) => sum + f.size, new Blob([text]).size)
  if (total > CAPTURE_MAX) return `The share is larger than ${bytes(CAPTURE_MAX)}.`

  return ""
}

function bytes(n) {
  return n >= MIB ? `${Math.floor(n / MIB)} MiB` : `${Math.ceil(n / KIB)} KiB`
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

// keep stores the capture page and the assets it loaded. Only assets are
// kept: a URL under /static/ of this origin, or of the origin the page loads
// its assets from (STATIC_CDN).
async function keep(urls) {
  const cache = await caches.open(CACHE)
  const roots = [new URL(STATIC, self.location.origin).href]

  try {
    const resp = await fetch(PAGE, { credentials: "same-origin" })
    if (resp.ok && !resp.redirected) {
      roots.push(...assetRoots(await resp.clone().text()))
      await cache.put(PAGE, resp)
    }
  } catch (_) {
    // offline or refused: kept on the next visit
  }

  for (const url of urls) {
    let abs

    try {
      abs = new URL(url, self.location.origin)
    } catch (_) {
      continue
    }

    if (!roots.some((r) => abs.href.startsWith(r))) continue

    try {
      const resp = await fetch(abs.href, { credentials: "same-origin" })
      if (resp.ok && !resp.redirected) await cache.put(abs.href, resp)
    } catch (_) {
      // offline or refused: kept on the next visit
    }
  }
}

// assetRoots are the /static/ roots of the scripts and styles the capture
// page names.
function assetRoots(html) {
  const roots = []

  for (const m of html.matchAll(/\b(?:src|href)="([^"]*?\/static\/)/g)) {
    try {
      roots.push(new URL(m[1], self.location.origin).href)
    } catch (_) {
      // not a URL
    }
  }

  return roots
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
