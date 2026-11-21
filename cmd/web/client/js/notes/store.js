// The IndexedDB store of the capture page (capture.js) and its service worker
// (sw.js): one record per capture, queued until the worker uploads it.
//
// A record: id, savedAt (ms), capturedAt (ISO 8601), csrf, status ("queued",
// "uploaded" or "failed"), kind ("note" or "share"), text, images (Blobs),
// audio (a Blob or null), and for the last-capture card seconds, photos,
// voice, error, noteId and deletePrompt.

const DB_NAME = "pcom-notes"
const DB_VERSION = 1
const STORE = "captures"

// SYNC_TAG is the background sync the page registers and the worker answers.
export const SYNC_TAG = "notes-upload"
// EXCERPT_LENGTH is how much of a capture's text the last-capture card shows.
export const EXCERPT_LENGTH = 80

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
export async function withStore(mode, fn) {
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

// records returns every record.
export function records() {
  return withStore("readonly", (s) => s.getAll())
}
