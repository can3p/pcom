// The capture page's work, driven by capture/notes_capture_controller.js in
// the page's own bundle (../capture.js).
//
// Stop is save: a recording, a photo or typed text is written to IndexedDB at
// once and the service worker (pkg/web/app/notes_sw.js, which shares the
// record format) uploads it with retries. The page shows the last capture and
// whether it is still on the phone or saved.

const DB_NAME = "pcom-notes"
const DB_VERSION = 1
const STORE = "captures"
const WORKER = "/controls/notes/sw.js"
const SCOPE = "/controls/notes/"
const SYNC_TAG = "notes-upload"
const DELETE = "/controls/action/note_delete"
const FOCUS = "/controls/action/note_focus"
// the containers the server keeps, the first one the browser records wins
const AUDIO_TYPES = ["audio/webm;codecs=opus", "audio/webm", "audio/mp4"]
const JPEG_QUALITY = 0.85
const TICK_MS = 250
const EXCERPT_LENGTH = 80
// the query parameter the share target's redirect names its note in
const SHARED_PARAM = "shared"

const STATUS_QUEUED = "Saved on the phone, uploading…"
const STATUS_SAVED = "Saved"

export class Capture {
  constructor(c) {
    this.c = c
    this.gone = false

    this.onMessage = () => this.render()
    this.onOnline = () => this.kick()
    this.onVisible = () => {
      // the browser drops the lock when the page is hidden
      if (document.visibilityState === "visible" && this.recording) this.lockScreen()
    }

    navigator.serviceWorker?.addEventListener("message", this.onMessage)
    window.addEventListener("online", this.onOnline)
    document.addEventListener("visibilitychange", this.onVisible)

    c.limitTarget.textContent = clock(c.maxSecondsValue)

    this.begin(c.startValue)
    this.init()
  }

  disconnect() {
    this.gone = true
    navigator.serviceWorker?.removeEventListener("message", this.onMessage)
    window.removeEventListener("online", this.onOnline)
    document.removeEventListener("visibilitychange", this.onVisible)

    // leaving the page mid-recording keeps what was said
    this.stop()
  }

  async init() {
    await this.register()
    await refreshCsrf(this.c.csrfValue)
    await this.adoptShared()
    await this.render()
    this.kick()
  }

  // adoptShared keeps the note the share target saved (?shared=) as the last
  // capture, already uploaded, and takes the parameter off the URL, so a
  // reload doesn't bring it back after a delete.
  async adoptShared() {
    const note = this.c.sharedValue
    if (!note?.noteId) return

    const rec = { id: "shared-" + note.noteId, savedAt: Date.now(), status: "uploaded", images: [], audio: null, seconds: 0, ...note }
    this.shared = rec

    try {
      await withStore("readwrite", (s) => s.put(rec))
      await prune(rec.id)
    } catch (_) {
      // no IndexedDB: render shows it from memory
    }

    const url = new URL(window.location.href)
    url.searchParams.delete(SHARED_PARAM)
    window.history.replaceState(window.history.state, "", url)
  }

  async register() {
    if (!navigator.serviceWorker) return

    try {
      await navigator.serviceWorker.register(WORKER, { scope: SCOPE })
      const reg = await navigator.serviceWorker.ready
      reg.active?.postMessage({ type: "keep", urls: assetURLs() })
    } catch (err) {
      console.warn("notes: no service worker", err)
    }
  }

  // kick asks the service worker to upload what is queued now, and the
  // browser to wake it when the network is back.
  async kick() {
    if (!navigator.serviceWorker) return

    const reg = await navigator.serviceWorker.ready
    reg.active?.postMessage({ type: "flush" })

    try {
      await reg.sync?.register(SYNC_TAG)
    } catch (_) {
      // no background sync: the page's online event and its next visit flush
    }
  }

  // begin starts the mode ?start= asks for; when the browser wants a tap
  // first, one big button does it.
  async begin(start) {
    if (start === "voice") {
      if (!(await this.dictate([], new Date(), true))) this.offerTap(start, "Tap to dictate")
    } else if (start === "photo") {
      if (navigator.userActivation && !navigator.userActivation.isActive) {
        this.offerTap(start, "Tap to take a photo")
      } else {
        this.photo(false)
      }
    }
  }

  offerTap(mode, label) {
    this.tapMode = mode
    this.c.tapTarget.textContent = label
    this.c.tapTarget.hidden = false
  }

  tapStart() {
    this.c.tapTarget.hidden = true

    if (this.tapMode === "voice") this.dictate()
    if (this.tapMode === "photo") this.photo(false)
  }

  // dictate records until Stop or the time limit; images are the photos the
  // recording is a caption of. It says whether the recording started.
  async dictate(images = [], capturedAt = new Date(), quiet = false) {
    if (this.recording) return true

    this.hideError()

    let stream

    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: true })
    } catch (err) {
      if (!quiet) this.showError("The microphone is not available: " + err.message)

      return false
    }

    const type = AUDIO_TYPES.find((t) => window.MediaRecorder?.isTypeSupported(t))
    const recorder = new MediaRecorder(stream, type ? { mimeType: type } : undefined)
    const r = { recorder, stream, chunks: [], images, capturedAt, startedAt: Date.now() }

    recorder.ondataavailable = (e) => {
      if (e.data.size) r.chunks.push(e.data)
    }
    recorder.onstop = () => this.recorded(r)

    this.recording = r
    recorder.start(1000)

    this.c.timerTarget.textContent = clock(0)
    this.c.recHintTarget.textContent = images.length ? "Recording a caption for the photo." : ""
    this.show("recorder")
    this.ticker = setInterval(() => this.tick(), TICK_MS)
    this.lockScreen()

    return true
  }

  tick() {
    const seconds = Math.floor((Date.now() - this.recording.startedAt) / 1000)
    this.c.timerTarget.textContent = clock(seconds)

    if (seconds >= this.c.maxSecondsValue) this.stop()
  }

  stop() {
    const r = this.recording
    if (!r || r.recorder.state === "inactive") return

    clearInterval(this.ticker)
    r.recorder.stop()
  }

  async recorded(r) {
    clearInterval(this.ticker)
    this.recording = null
    r.stream.getTracks().forEach((t) => t.stop())
    this.releaseScreen()

    if (!this.gone) this.show("modes")

    const audio = new Blob(r.chunks, { type: r.recorder.mimeType || r.chunks[0]?.type || "audio/webm" })

    if (!audio.size && !r.images.length) {
      this.showError("Nothing was recorded.")
      return
    }

    await this.save({
      capturedAt: r.capturedAt.toISOString(),
      images: r.images,
      audio: audio.size ? audio : null,
      seconds: Math.round((Date.now() - r.startedAt) / 1000),
    })
  }

  photo(withVoice) {
    this.withVoice = withVoice
    this.c.cameraTarget.value = ""
    this.c.cameraTarget.click()
  }

  async photoTaken() {
    const files = [...this.c.cameraTarget.files]
    if (!files.length) return

    this.hideError()

    const capturedAt = new Date()
    let images

    try {
      images = await Promise.all(files.map((f) => scale(f, this.c.maxDimensionValue)))
    } catch (_) {
      this.showError("This photo can't be read.")
      return
    }

    if (this.withVoice && (await this.dictate(images, capturedAt))) return

    await this.save({ capturedAt: capturedAt.toISOString(), images })
  }

  type() {
    this.hideError()
    this.show("typer")
    this.c.textTarget.focus()
  }

  cancelText() {
    this.show("modes")
  }

  async saveText() {
    const text = this.c.textTarget.value

    if (!text.trim()) {
      this.showError("Write something first.")
      return
    }

    await this.save({ text })
    this.c.textTarget.value = ""
    this.show("modes")
  }

  // save queues a capture and shows it as the last one.
  async save(fields) {
    const rec = {
      id: crypto.randomUUID(),
      savedAt: Date.now(),
      capturedAt: new Date().toISOString(),
      csrf: this.c.csrfValue,
      status: "queued",
      text: "",
      images: [],
      audio: null,
      seconds: 0,
      ...fields,
    }
    rec.photos = rec.images.length
    rec.voice = !!rec.audio

    try {
      await withStore("readwrite", (s) => s.put(rec))
      await prune(rec.id)
    } catch (err) {
      this.showError("This browser can't keep the note: " + err.message)
      return
    }

    await this.render()
    this.kick()
  }

  async render() {
    if (this.gone) return

    const rec = (await latest()) || this.shared
    const c = this.c

    this.last = rec
    c.cardTarget.hidden = !rec
    if (!rec) return

    c.cardWhatTarget.textContent = describe(rec)
    if (rec.voice && !rec.photos && !rec.seconds) this.measure(rec)

    const s = c.cardStatusTarget
    s.classList.toggle("ok", rec.status === "uploaded")
    s.classList.toggle("err", rec.status === "failed")

    if (rec.status === "uploaded") s.textContent = STATUS_SAVED
    else if (rec.status === "failed") s.textContent = "Not saved: " + rec.error
    else s.textContent = STATUS_QUEUED
  }

  // deleteLast drops the last capture from the queue, or deletes its note
  // once it is uploaded.
  async deleteLast() {
    const rec = this.last
    if (!rec) return

    this.hideError()

    if (rec.status === "uploaded") {
      if (!window.confirm(rec.deletePrompt)) return

      let resp

      try {
        resp = await fetch(DELETE, {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-CSRFToken": this.c.csrfValue },
          body: JSON.stringify({ noteId: rec.noteId }),
        })
      } catch (_) {
        this.showError("Not deleted: no network.")
        return
      }

      if (!resp.ok) {
        this.showError("Not deleted: " + ((await resp.json().catch(() => ({}))).explanation || resp.statusText))
        return
      }
    }

    if (rec.id === this.shared?.id) this.shared = null
    await withStore("readwrite", (s) => s.delete(rec.id))
    await this.render()
  }

  // measure puts the length of a recording on the card once it is known: a
  // shared recording comes without one. It reads the recording itself (the
  // queued file, or the uploaded note's), and the card keeps no length when
  // the browser can't decode it.
  async measure(rec) {
    this.lengths ??= new Map()

    const show = (seconds) => {
      if (seconds > 0 && this.last === rec && !this.gone) {
        this.c.cardWhatTarget.textContent = describe({ ...rec, seconds })
      }
    }

    if (this.lengths.has(rec.id)) {
      show(this.lengths.get(rec.id))
      return
    }

    this.lengths.set(rec.id, 0)

    let ctx

    try {
      const data = rec.audio
        ? await rec.audio.arrayBuffer()
        : await (await fetch(`/controls/notes/${rec.noteId}/audio`)).arrayBuffer()

      ctx = new AudioContext()
      const seconds = Math.round((await ctx.decodeAudioData(data)).duration)

      this.lengths.set(rec.id, seconds)
      show(seconds)
    } catch (_) {
      // no length
    } finally {
      ctx?.close()
    }
  }

  // stopFocus clears the focus tag and reloads the page, which shows it gone.
  async stopFocus() {
    this.hideError()

    try {
      const resp = await fetch(FOCUS, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRFToken": this.c.csrfValue },
        body: JSON.stringify({ tag: "" }),
      })

      if (!resp.ok) {
        this.showError("Not changed: " + ((await resp.json().catch(() => ({}))).explanation || resp.statusText))
        return
      }
    } catch (_) {
      this.showError("Not changed: no network.")
      return
    }

    window.location.reload()
  }

  // show leaves one of the page's panels visible: the mode buttons, the
  // recorder or the text box.
  show(panel) {
    const c = this.c
    c.modesTarget.hidden = panel !== "modes"
    c.recorderTarget.hidden = panel !== "recorder"
    c.typerTarget.hidden = panel !== "typer"
  }

  showError(msg) {
    this.c.errorTarget.textContent = msg
    this.c.errorTarget.hidden = false
  }

  hideError() {
    this.c.errorTarget.hidden = true
  }

  async lockScreen() {
    try {
      this.wakeLock = await navigator.wakeLock?.request("screen")
    } catch (_) {
      // refused (low battery, hidden page): recording goes on
    }
  }

  releaseScreen() {
    this.wakeLock?.release().catch(() => {})
    this.wakeLock = null
  }
}

// scale draws a photo at most max pixels on its longest side and encodes it
// as JPEG, which also turns a HEIC the browser can read into one the server
// keeps.
async function scale(file, max) {
  const bitmap = await createImageBitmap(file)
  const ratio = Math.min(1, max / Math.max(bitmap.width, bitmap.height))
  const canvas = document.createElement("canvas")
  canvas.width = Math.round(bitmap.width * ratio)
  canvas.height = Math.round(bitmap.height * ratio)

  const ctx = canvas.getContext("2d")
  ctx.fillStyle = "#fff" // JPEG has no transparency
  ctx.fillRect(0, 0, canvas.width, canvas.height)
  ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
  bitmap.close()

  return new Promise((resolve, reject) => {
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error("no JPEG"))), "image/jpeg", JPEG_QUALITY)
  })
}

// describe names a capture by what it holds; an uploaded one no longer holds
// its files, so it goes by the counts saved with it.
function describe(rec) {
  const photos = rec.photos === 1 ? "A photo" : `${rec.photos} photos`

  if (rec.text) {
    const t = rec.text.trim()
    return t.length > EXCERPT_LENGTH ? t.slice(0, EXCERPT_LENGTH) + "…" : t
  }

  if (rec.photos && rec.voice) return `${photos} with a voice caption`
  if (rec.photos) return photos

  return rec.seconds ? `A recording of ${clock(rec.seconds)}` : "A recording"
}

function clock(seconds) {
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m}:${String(s).padStart(2, "0")}`
}

// assetURLs are the scripts, styles and fonts the page loaded, for the
// service worker to keep.
function assetURLs() {
  return performance.getEntriesByType("resource")
    .map((e) => e.name)
    .filter((u) => /\.(js|css|woff2)(\?|$)/.test(u) && !u.endsWith(WORKER))
}

function openDB() {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION)
    req.onupgradeneeded = () => req.result.createObjectStore(STORE, { keyPath: "id" })
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}

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

// eachRecord calls fn with every record and its cursor, in one transaction.
function eachRecord(fn) {
  return withStore("readwrite", (s) => {
    const req = s.openCursor()
    req.onsuccess = () => {
      const cur = req.result
      if (!cur) return

      fn(cur.value, cur)
      cur.continue()
    }

    return null
  })
}

// prune keeps the queued captures and the last one.
function prune(keepID) {
  return eachRecord((rec, cur) => {
    if (rec.id !== keepID && rec.status !== "queued") cur.delete()
  })
}

// refreshCsrf gives the queued captures this session's token, which the
// service worker sends with them.
async function refreshCsrf(token) {
  try {
    await eachRecord((rec, cur) => {
      if (rec.status === "queued" && rec.csrf !== token) cur.update({ ...rec, csrf: token })
    })
  } catch (_) {
    // no IndexedDB: save says so
  }
}

async function latest() {
  try {
    const recs = await withStore("readonly", (s) => s.getAll())
    return recs.sort((a, b) => b.savedAt - a.savedAt)[0]
  } catch (_) {
    return undefined
  }
}
