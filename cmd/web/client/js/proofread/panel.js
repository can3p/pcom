// The grammar check and style review panel of the post editor, loaded by
// controllers/proofread_controller.js when an editor with the feature appears
// (a separate chunk: only the editor pays for it). The browser
// talks only to pcom (/controls/action/proofread_*); the text is sent when a
// button is pressed and nothing runs as the author types.
//
// Grammar suggestions are quotes with a replacement. The controller finds each
// once when the answer arrives and then follows the edits: on every `input`
// the old and new value give one edited range (common prefix and suffix), the
// suggestions after it shift, and one that touches it is kept when its text is
// still there, searched for nearby, or marked stale. This is the approach of
// Harper's lint-framework spanMapping.ts (Apache-2.0).
//
// The post form replaces itself after a validation error, which connects a new
// controller (before the old one disconnects). The state is shared through
// `kept`, keyed by post id, and every suggestion is located again by its quote.
//
// The panel is markup built from strings (every page pays for this file on its
// first visit, so it stays small); one click handler on the panel dispatches
// on data-act.

const kept = new Map()
const KEPT_FOR_MS = 10000
const NEARBY = 200
const RENDER_DELAY_MS = 150

const STALE_TEXT = "You changed this text after the grammar check, so the suggestion can no longer be placed or applied."
const LOST_TEXT = "This passage was rewritten after the review, so the note can't be shown in the text. It was about:"

let nextID = 1

const esc = (s) => String(s).replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`)
const cap = (s) => (s[0].toUpperCase() + s.slice(1)).replace("_", " ")
const plural = (n, w) => `${n} ${w}${n === 1 ? "" : "s"}`

const btn = (act, id, label, cls = "btn btn-secondary", more = "") =>
  `<button type="button" class="${cls}" data-act="${act}" data-id="${id}" ${more}>${label}</button>`

const ago = (iso) => {
  const sec = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000)

  if (sec < 45) return "just now"
  if (sec < 3600) return `${plural(Math.max(1, Math.round(sec / 60)), "minute")} ago`
  if (sec < 86400) return `${plural(Math.round(sec / 3600), "hour")} ago`

  return `${plural(Math.round(sec / 86400), "day")} ago`
}

// diffRange: the one range that differs between a and b.
const diffRange = (a, b) => {
  const max = Math.min(a.length, b.length)
  let p = 0
  let s = 0

  while (p < max && a[p] === b[p]) p++
  while (s < max - p && a[a.length - 1 - s] === b[b.length - 1 - s]) s++

  return { p, oldEnd: a.length - s, newEnd: b.length - s }
}

// locate: the start of the quote in the occurrence-th (from zero)
// non-overlapping match of before+quote, or -1.
const locate = (text, before, quote, occurrence) => {
  const needle = before + quote
  let from = 0
  let pos = -1

  for (let i = 0; i <= occurrence; i++) {
    pos = text.indexOf(needle, from)
    if (pos < 0) return -1
    from = pos + needle.length
  }

  return pos + before.length
}

// nearest: the occurrence of s in text closest to pos, within NEARBY, or -1.
const nearest = (text, s, pos) => {
  let best = -1

  for (let i = text.indexOf(s); i >= 0; i = text.indexOf(s, i + 1)) {
    if (best < 0 || Math.abs(i - pos) < Math.abs(best - pos)) best = i
  }

  return best >= 0 && Math.abs(best - pos) <= NEARBY ? best : -1
}

const api = async (method, url, body) => {
  let headers = {}

  try {
    headers = JSON.parse(document.body.getAttribute("hx-headers") || "{}")
  } catch (e) {
    // no CSRF header: the server answers with its own message
  }

  if (body) headers["Content-Type"] = "application/json"

  let resp

  try {
    resp = await fetch(url, { method, headers, body: body && JSON.stringify(body) })
  } catch (e) {
    throw new Error("Network error - please check your connection")
  }

  const data = await resp.json().catch(() => ({}))

  if (!resp.ok) throw new Error(data.explanation || `The request failed (${resp.status})`)

  return data
}

const freshState = () => ({
  tab: "grammar",
  collapsed: false,
  remaining: null,
  notice: "",
  selected: null,
  grammar: { items: [], status: "idle", error: "", checkedAt: null },
  style: { status: "idle", error: "", review: null, earlier: [], showEarlier: false, keeping: null, keepReason: "", nextNote: "", selected: null },
  dismissedMemory: [],
})

// Panel is the proofread controller's behaviour; c is the Stimulus controller
// that owns the element (targets, values).
export class Panel {
  constructor(c) {
    this.c = c
    this.element = c.element
    this.connect()
  }

  connect() {
    this.alive = true
    this.form = this.element.querySelector("form")
    this.fields = {
      subject: this.form.querySelector("[name=subject]"),
      body: this.form.querySelector("textarea[name=body]"),
    }
    this.last = { subject: this.fields.subject.value, body: this.fields.body.value }
    this.postID = this.readPostID()

    // htmx puts the new form in place before it removes the old one, so the
    // old controller may still be connected: its state is taken over then too
    const parked = kept.get(this.stateKey(this.postID))
    const taken = parked && (parked.controller.alive || Date.now() - parked.at < KEPT_FOR_MS)

    this.s = taken ? parked.state : freshState()
    this.s.owner = this // an answer that arrives later finds the live controller
    kept.set(this.stateKey(this.postID), { state: this.s, controller: this, at: 0 })

    if (taken) this.relocateAll()

    const box = this.fields.body.closest(".mdeditor")
    this.layer = Object.assign(document.createElement("div"), { className: "pr-hl" })
    this.mirror = Object.assign(document.createElement("div"), { className: "pr-mirror" })
    box.insertBefore(this.layer, this.fields.body)
    box.append(this.mirror)

    const on = (el, ev, fn) => {
      (this.listeners ||= []).push([el, ev, fn])
      el.addEventListener(ev, fn)
    }

    for (const f of ["subject", "body"]) {
      on(this.fields[f], "input", () => this.onInput(f))
      on(this.fields[f], "click", () => this.onCaret(f))
    }

    on(this.fields.body, "keyup", (e) => e.key.startsWith("Arrow") && this.onCaret("body"))
    on(this.fields.body, "scroll", () => this.drawHighlight())
    on(this.c.panelTarget, "click", (e) => {
      const b = e.target.closest("[data-act]")
      if (b) this.act(b.dataset.act, b.dataset.id)
    })
    on(this.c.panelTarget, "input", (e) => {
      const st = this.s.style
      if (e.target.dataset.key === "keep") st.keepReason = e.target.value
      else if (e.target.dataset.key === "next") st.nextNote = e.target.value
    })

    this.observer = new MutationObserver(() => this.syncPostID())
    this.observer.observe(this.form, { childList: true, subtree: true })

    this.render()

    if (this.postID && !this.s.style.loaded) this.loadStyle()
  }

  disconnect() {
    this.alive = false
    clearTimeout(this.renderTimer)
    this.observer.disconnect()

    for (const [el, ev, fn] of this.listeners) el.removeEventListener(ev, fn)

    this.layer.remove()
    this.mirror.remove()

    // park the state for a form that is about to take its place, unless it has already
    const key = this.stateKey(this.readPostID())
    const entry = kept.get(key)

    if (!entry || !entry.controller.alive || entry.controller === this) {
      kept.set(key, { state: this.s, controller: this, at: Date.now() })
    }
  }

  // ---- the post id and the stored dismissals --------------------------------

  readPostID() {
    return this.element.querySelector("input[name=post_id]")?.value || ""
  }

  stateKey(id) {
    return id || "new"
  }

  syncPostID() {
    const id = this.readPostID()
    const old = this.postID

    if (id === old) return

    this.postID = id

    // the first autosave gave the draft an id: its dismissals and state follow it
    if (!old && id) {
      const list = this.readDismissed("")

      this.writeDismissed([], "")
      if (list.length) this.writeDismissed(list.concat(this.readDismissed(id)), id)
    }

    const entry = kept.get(this.stateKey(old))

    if (entry && entry.controller === this) {
      kept.delete(this.stateKey(old))
      kept.set(this.stateKey(id), entry)
    }

    this.render()
  }

  storageKey(id) {
    return `pcom:proofread:dismissed:${this.stateKey(id)}`
  }

  readDismissed(id = this.postID) {
    try {
      return JSON.parse(localStorage.getItem(this.storageKey(id)) || "[]")
    } catch (e) {
      return this.s.dismissedMemory
    }
  }

  writeDismissed(list, id = this.postID) {
    this.s.dismissedMemory = list

    try {
      if (list.length) localStorage.setItem(this.storageKey(id), JSON.stringify(list))
      else localStorage.removeItem(this.storageKey(id))
    } catch (e) {
      // without storage the dismissals last until the page is left
    }
  }

  // ---- toolbar actions --------------------------------------------------------

  async checkGrammar() {
    const g = this.s.grammar

    if (g.status === "loading") return

    const snapshot = { subject: this.fields.subject.value, body: this.fields.body.value }

    Object.assign(this.s, { tab: "grammar", collapsed: false })
    Object.assign(g, { status: "loading", error: "" })
    this.render()

    try {
      const res = await api("POST", this.c.grammarUrlValue, { post_id: this.postID, ...snapshot })
      const dismissed = this.readDismissed()

      this.s.remaining = res.remaining
      g.items = res.suggestions.flatMap((x) => {
        const start = locate(snapshot[x.field] ?? "", x.before, x.quote, x.occurrence)

        if (start < 0) return []

        const gone = dismissed.some((d) => d.field === x.field && d.quote === x.quote && d.replacement === x.replacement)

        return [{ ...x, id: nextID++, start, end: start + x.quote.length, state: gone ? "dismissed" : "open" }]
      })
      g.checkedAt = new Date().toISOString()
      g.status = "idle"

      // the author may have kept typing: follow those edits
      const me = this.s.owner

      for (const f of ["subject", "body"]) {
        if (snapshot[f] !== me.fields[f].value) me.remap(f, snapshot[f], me.fields[f].value)
      }

      this.s.selected = g.items.find((i) => i.state === "open")?.id ?? null
    } catch (e) {
      this.fail(g, e, "grammar")
    }

    this.s.owner.render()
  }

  async reviewStyle() {
    const st = this.s.style

    if (st.status === "loading" || !this.postID) return

    Object.assign(this.s, { tab: "style", collapsed: false })
    Object.assign(st, { status: "loading", error: "" })
    this.render()

    try {
      const res = await api("POST", this.c.styleUrlValue, {
        post_id: this.postID,
        subject: this.fields.subject.value,
        body: this.fields.body.value,
        author_note: st.nextNote,
      })

      if (st.review) st.earlier.unshift({ number: st.review.number, summary: st.review.summary, created_at: st.review.created_at })

      this.s.remaining = res.remaining
      Object.assign(st, { review: res.review, loaded: true, nextNote: "", keeping: null, selected: null, status: "idle" })
    } catch (e) {
      this.fail(st, e, "style")
    }

    this.s.owner.render()
  }

  // fail keeps the message; one that says the hour's checks are used up also disables the button
  fail(part, e, kind) {
    Object.assign(part, { status: "error", error: e.message })

    if (/^You have used all/.test(e.message) && this.s.remaining) this.s.remaining[kind] = 0
  }

  async loadStyle() {
    try {
      const res = await api("GET", this.c.stateUrlValue + this.postID)

      this.s.remaining = res.remaining
      Object.assign(this.s.style, { review: res.review, earlier: res.earlier || [], loaded: true })
    } catch (e) {
      this.s.notice = e.message
    }

    this.s.owner.render()
  }

  // ---- following the edits ------------------------------------------------------

  onInput(field) {
    const old = this.last[field]

    this.last[field] = this.fields[field].value
    this.remap(field, old, this.last[field])
    this.drawHighlight()
    clearTimeout(this.renderTimer)
    this.renderTimer = setTimeout(() => this.render(), RENDER_DELAY_MS)
  }

  remap(field, old, neu) {
    const { p, oldEnd, newEnd } = diffRange(old, neu)
    const delta = newEnd - oldEnd

    for (const it of this.s.grammar.items) {
      if (it.field !== field || it.state === "stale" || it === this.applying || it.end <= p) continue

      if (it.start >= oldEnd) {
        it.start += delta
        it.end += delta
        continue
      }

      const cur = it.state === "applied" ? it.replacement : it.quote
      const guess = it.start < p ? it.start : it.start + delta
      const at = neu.substr(guess, cur.length) === cur ? guess : nearest(neu, cur, guess)

      if (at < 0) it.state = "stale"
      else Object.assign(it, { start: at, end: at + cur.length })
    }
  }

  // relocateAll finds every suggestion again by its text after the form was replaced.
  relocateAll() {
    for (const it of this.s.grammar.items) {
      const value = this.fields[it.field].value
      const cur = it.state === "applied" ? it.replacement : it.quote

      if (it.state === "stale" || value.substr(it.start, cur.length) === cur) continue

      const at = nearest(value, cur, it.start)

      if (at < 0) it.state = "stale"
      else Object.assign(it, { start: at, end: at + cur.length })
    }
  }

  // ---- apply, undo, dismiss -----------------------------------------------------

  // replace puts text into the item's range the way typing would, so the
  // browser's own undo works, and lets autosave know. False: the text there changed.
  replace(it, text) {
    const el = this.fields[it.field]
    const cur = it.state === "applied" ? it.replacement : it.quote

    if (el.value.substr(it.start, cur.length) !== cur) return false

    el.focus()
    el.setSelectionRange(it.start, it.end)
    this.applying = it

    if (!document.execCommand("insertText", false, text)) {
      el.setRangeText(text, it.start, it.end, "end")
      el.dispatchEvent(new Event("input", { bubbles: true }))
    }

    this.applying = null
    it.end = it.start + text.length
    window.htmx.trigger(el, "change")

    return true
  }

  apply(it) {
    if (it.state === "open") {
      it.state = this.replace(it, it.replacement) ? "applied" : "stale"
      this.clearHighlight()
    }
  }

  undo(it) {
    if (it.state === "applied") it.state = this.replace(it, it.quote) ? "open" : "stale"
  }

  dismiss(it) {
    this.writeDismissed([...this.readDismissed(), { field: it.field, quote: it.quote, replacement: it.replacement }])
    it.state = "dismissed"

    if (this.s.selected === it.id) this.s.selected = this.s.grammar.items.find((i) => i.state === "open" || i.state === "stale")?.id ?? null

    this.clearHighlight()
  }

  showDismissedAgain() {
    this.writeDismissed([])

    for (const it of this.s.grammar.items) {
      if (it.state === "dismissed") it.state = "open"
    }

    this.relocateAll()
  }

  // ---- selecting and highlighting -----------------------------------------------

  selectItem(it) {
    this.s.selected = it.id
    this.reveal(it.state === "stale" ? null : it)
  }

  step(dir) {
    const shown = this.s.grammar.items.filter((i) => i.state !== "dismissed")
    const at = shown.findIndex((i) => i.id === this.s.selected)

    if (shown.length) this.selectItem(shown[(at + dir + shown.length) % shown.length])
  }

  onCaret(field) {
    const el = this.fields[field]
    const pos = el.selectionStart
    const it = this.s.grammar.items.find((i) => i.field === field && i.state === "open" && i.start <= pos && pos < i.end)

    if (el.selectionStart !== el.selectionEnd || !it || this.s.selected === it.id) return

    Object.assign(this.s, { tab: "grammar", selected: it.id })
    this.render()
    this.draw(it, false)
    this.c.panelTarget.querySelector(".pr-card.is-selected")?.scrollIntoView({ block: "nearest" })
  }

  // reveal scrolls the field to the passage and tints it; an item without a
  // place in the text only clears the tint.
  reveal(it) {
    this.clearHighlight()
    if (it) this.draw(it, true)
  }

  clearHighlight() {
    this.hlRects = null
    this.layer.replaceChildren()
    this.fields.subject.classList.remove("pr-mark")
  }

  // draw tints the range of {field, start, end}; a hidden mirror of the
  // textarea measures it. The browser's selection is not used: the next
  // keypress would replace the selected text.
  draw({ field, start, end }, scroll) {
    this.clearHighlight()

    if (field === "subject") this.fields.subject.classList.add("pr-mark")

    if (field !== "body") return

    const ta = this.fields.body
    const cs = getComputedStyle(ta)
    const span = Object.assign(document.createElement("span"), { textContent: ta.value.slice(start, end) || "​" })

    for (const p of ["fontFamily", "fontSize", "fontWeight", "fontStyle", "lineHeight", "letterSpacing", "wordSpacing",
      "textTransform", "textIndent", "tabSize", "paddingTop", "paddingRight", "paddingBottom", "paddingLeft"]) {
      this.mirror.style[p] = cs[p]
    }

    this.mirror.style.width = `${ta.clientWidth}px`
    this.mirror.replaceChildren(ta.value.slice(0, start), span)

    const origin = this.mirror.getBoundingClientRect()

    this.hlRects = Array.from(span.getClientRects()).map((r) => ({ top: r.top - origin.top, left: r.left - origin.left, width: r.width, height: r.height }))

    const first = this.hlRects[0]

    if (scroll && first) {
      if (first.top < ta.scrollTop || first.top + first.height > ta.scrollTop + ta.clientHeight) ta.scrollTop = Math.max(0, first.top - ta.clientHeight / 3)

      ta.scrollIntoView({ block: "nearest" })
    }

    this.drawHighlight()
  }

  // drawHighlight places the measured boxes for the textarea's scroll position.
  drawHighlight() {
    if (!this.hlRects) return

    const ta = this.fields.body

    Object.assign(this.layer.style, { top: `${ta.offsetTop}px`, left: `${ta.offsetLeft}px`, width: `${ta.clientWidth}px`, height: `${ta.clientHeight}px` })

    this.layer.replaceChildren(...this.hlRects.map((r) => {
      const box = Object.assign(document.createElement("div"), { className: "pr-hl-box" })

      Object.assign(box.style, { top: `${r.top - ta.scrollTop}px`, left: `${r.left}px`, width: `${r.width}px`, height: `${r.height}px` })

      return box
    }))
  }

  // ---- style notes ---------------------------------------------------------------

  // noteRange: where the note's passage is now, null for a note about no
  // passage, false when it can't be found any more.
  noteRange(n) {
    if (n.field !== "subject" && n.field !== "body") return null

    const text = this.fields[n.field].value
    const at = locate(text, n.before, n.quote, n.occurrence)
    const start = at < 0 ? text.indexOf(n.quote) : at

    return start < 0 ? false : { field: n.field, start, end: start + n.quote.length }
  }

  async post(url, body) {
    try {
      await api("POST", url, body)
      this.s.notice = ""

      return true
    } catch (e) {
      this.s.notice = e.message

      return false
    }
  }

  async setNote(n, status, reason) {
    if (await this.post(this.c.noteUrlValue, { note_id: n.id, status, reason })) {
      Object.assign(n, { status, keep_reason: reason })
      this.s.style.keeping = null
    }

    this.s.owner.render()
  }

  async startFresh() {
    if (!window.confirm("Delete all style reviews of this draft? The notes and your reasons are lost.")) return

    if (await this.post(this.c.resetUrlValue, { post_id: this.postID })) {
      Object.assign(this.s.style, { review: null, earlier: [], showEarlier: false, keeping: null, selected: null, nextNote: "" })
    }

    this.s.owner.render()
  }

  // ---- clicks in the panel ----------------------------------------------------------

  act(name, id) {
    const s = this.s
    const st = s.style
    const it = s.grammar.items.find((i) => String(i.id) === id)
    const note = st.review?.notes.find((n) => n.id === id)

    const acts = {
      tab: () => { s.tab = id },
      bar: () => { s.collapsed = !s.collapsed },
      earlier: () => { st.showEarlier = !st.showEarlier },
      select: () => this.selectItem(it),
      apply: () => this.apply(it),
      undo: () => this.undo(it),
      dismiss: () => this.dismiss(it),
      again: () => this.showDismissedAgain(),
      prev: () => this.step(-1),
      next: () => this.step(1),
      retryGrammar: () => this.checkGrammar(),
      retryStyle: () => this.reviewStyle(),
      fresh: () => this.startFresh(),
      note: () => {
        st.selected = id
        this.reveal(this.noteRange(note) || (note.kind === "title" ? { field: "subject" } : null))
      },
      done: () => this.setNote(note, "done", ""),
      keep: () => Object.assign(st, { keeping: id, keepReason: "" }),
      keepSave: () => this.setNote(note, "kept", st.keepReason),
      cancel: () => { st.keeping = null },
    }

    acts[name]()
    this.render()

    if (name === "keep") this.c.panelTarget.querySelector("[data-key=keep]")?.focus()
  }

  // ---- rendering --------------------------------------------------------------------

  openCount(tab) {
    if (tab === "grammar") return this.s.grammar.items.filter((i) => i.state === "open" || i.state === "stale").length

    return (this.s.style.review?.notes || []).filter((n) => n.status === "open").length
  }

  render() {
    if (!this.alive) return

    const { s } = this
    const { grammar: g, style: st, remaining: r } = s
    const empty = g.checkedAt === null && g.status === "idle" && !st.review && st.status === "idle"
    const panel = this.c.panelTarget
    const scroll = panel.querySelector(".pr-list")?.scrollTop
    const active = document.activeElement
    const focusKey = panel.contains(active) ? active.dataset.key : null
    const caret = focusKey ? [active.selectionStart, active.selectionEnd] : null
    const noGrammar = r && r.grammar <= 0
    const noStyle = r && r.style <= 0

    // the limits line and the toolbar buttons
    this.c.limitsTarget.innerHTML = r
      ? `<span>Grammar: ${r.grammar} of ${r.grammar_limit} checks left this hour</span><span>Style: ${r.style} of ${r.style_limit} reviews left this hour</span>`
      : ""
    this.c.grammarButtonTarget.disabled = g.status === "loading" || !!noGrammar
    this.c.grammarButtonTarget.title = noGrammar ? "You have used all grammar checks for this hour" : ""
    this.c.styleButtonTarget.disabled = st.status === "loading" || !this.postID || !!noStyle
    this.c.styleButtonTarget.title = !this.postID ? "Available once the draft is saved" : (noStyle ? "You have used all style reviews for this hour" : "")

    panel.classList.toggle("is-empty", empty)
    panel.classList.toggle("is-collapsed", s.collapsed)
    this.element.classList.toggle("has-panel", !empty)

    const isG = s.tab === "grammar"
    const part = isG ? g : st
    const count = this.openCount(s.tab)
    const barText = part.status === "loading" ? (isG ? "Checking grammar…" : "Reviewing…")
      : part.status === "error" ? `The ${isG ? "grammar check" : "style review"} failed`
        : count ? plural(count, isG ? "grammar suggestion" : "style note") : (isG ? "No grammar suggestions" : "No open style notes")

    panel.innerHTML = `
      <button type="button" class="pr-bar" data-act="bar" aria-expanded="${!s.collapsed}"><span class="pr-grip"></span>${barText}</button>
      <div class="pr-head">
        <div class="pr-tabs" role="group" aria-label="Panel">
          ${["grammar", "style"].map((t) => `<button type="button" class="pr-tab" data-act="tab" data-id="${t}" aria-pressed="${s.tab === t}">${cap(t)} · ${this.openCount(t)}</button>`).join("")}
        </div>
        <p class="pr-sub">${this.sub()}</p>
      </div>
      <div class="pr-list">${s.notice ? `<p class="pr-error" role="alert">${esc(s.notice)}</p>` : ""}${isG ? this.grammarHTML() : this.styleHTML()}</div>
      ${isG ? "" : this.footHTML()}`

    panel.querySelector(".pr-list").scrollTop = scroll || 0

    // typed text lives in the state, not in the markup
    panel.querySelector("[data-key=keep]")?.append(st.keepReason)
    panel.querySelector("[data-key=next]")?.append(st.nextNote)

    const again = focusKey && panel.querySelector(`[data-key="${focusKey}"]`)

    if (again) {
      again.focus()
      again.setSelectionRange(...caret)
    }
  }

  sub() {
    const { grammar: g, style: st, tab } = this.s
    const r = st.review

    if (tab === "grammar") return g.checkedAt ? `Checked ${ago(g.checkedAt)}. Click a suggestion to see it in the text.` : "Press Check grammar to look for typos and punctuation mistakes."

    if (!r) return "Press Style review for notes on style, logic and pacing."

    return `Review ${r.number} of this draft · ${ago(r.created_at)} ${st.earlier.length ? btn("earlier", "", "Earlier reviews", "pr-link", `aria-expanded="${st.showEarlier}"`) : ""}`
  }

  // problem: the waiting or failed state of a request, with the retry button.
  problem(part, kind, retry) {
    const r = this.s.remaining
    const grammar = kind === "grammar"

    if (part.status === "loading") {
      return `<div class="pr-wait" role="status"><p>${grammar ? "Checking your text…" : "Reviewing…"}</p><span class="pr-skel"></span><span class="pr-skel"></span></div>
        <p class="pr-note">${grammar ? "You can keep typing. Suggestions for text you change meanwhile are matched again when they arrive." : "A style review takes a little while. You can keep typing."}</p>`
    }

    if (part.status !== "error") return ""

    return `<div class="pr-error" role="alert"><p>${esc(part.error)}</p>${r && r[kind] <= 0 ? "" : btn(retry, "", "Try again")}</div>`
  }

  grammarHTML() {
    const g = this.s.grammar
    const shown = g.items.filter((i) => i.state !== "dismissed")
    const dismissed = this.readDismissed().length
    let out = this.problem(g, "grammar", "retryGrammar")

    out += shown.map((it, at) => this.cardHTML(it, at, shown.length)).join("")

    if (!shown.length && g.status === "idle") {
      out += `<p class="pr-note">${g.checkedAt ? (g.items.length ? "All suggestions from this check were dismissed." : "No grammar issues found.") : "Suggestions appear here. Nothing in your text changes until you apply one."}</p>`
    }

    return out + (dismissed ? btn("again", "", `${dismissed} dismissed · show again`, "pr-link") : "")
  }

  cardHTML(it, at, total) {
    const sel = this.s.selected === it.id
    const acts = it.state === "stale"
      ? `<p class="pr-why">${STALE_TEXT}</p><div class="pr-acts">${btn("dismiss", it.id, "Dismiss")}</div>`
      : it.state === "applied"
        ? `<div class="pr-acts"><span class="status ok">Applied</span>${btn("undo", it.id, "Undo")}</div>`
        : sel ? `<p class="pr-why">${esc(it.explanation)}</p><div class="pr-acts">${btn("apply", it.id, "Apply", "btn btn-primary")}${btn("dismiss", it.id, "Dismiss")}</div>` : ""

    return `<div class="pr-card is-${it.state}${sel ? " is-selected" : ""}" data-kind="${it.kind}">
      <button type="button" class="pr-card-main" data-act="select" data-id="${it.id}" aria-expanded="${sel}">
        <span class="pr-kind"><span class="pr-dot"></span>${cap(it.kind)}<span class="pr-count">${at + 1} of ${total}</span></span>
        <span class="pr-fix"><s>${esc(it.quote)}</s> → <b>${esc(it.replacement)}</b></span>
      </button>${acts}
      ${sel && total > 1 ? `<div class="pr-pager">${btn("prev", "", "‹ Previous", "pr-link", 'aria-label="Previous suggestion"')}${btn("next", "", "Next ›", "pr-link", 'aria-label="Next suggestion"')}</div>` : ""}
    </div>`
  }

  styleHTML() {
    const st = this.s.style
    const r = st.review
    let out = this.problem(st, "style", "retryStyle")

    if (st.showEarlier) {
      out += `<div class="pr-earlier">${st.earlier.map((e) => `<div><span class="pr-kind">Review ${e.number} · ${ago(e.created_at)}</span><p class="pr-why">${esc(e.summary)}</p></div>`).join("")}</div>`
    }

    if (!r) {
      return out + (st.status !== "idle" ? "" : `<p class="pr-note">${this.postID ? "Notes on style, logic and pacing appear here. They never change your text: mark each one done, or keep the text as it is." : "A style review is kept with the post, so save the draft first."}</p>`)
    }

    const open = r.notes.filter((n) => n.status === "open")
    const closed = [["done", "done"], ["kept", "kept as is"]].map(([k, w]) => [r.notes.filter((n) => n.status === k).length, w]).filter(([n]) => n)

    out += `<div class="pr-summary"><span class="pr-kind">${r.number > 1 ? `Since review ${r.number - 1}` : "Overall"}</span><p class="pr-why">${esc(r.summary)}</p></div>`
    out += open.map((n) => this.noteHTML(n)).join("") || '<p class="pr-note">No open notes.</p>'

    return out + (closed.length ? `<p class="pr-note">${closed.map(([n, w]) => `${n} ${w}`).join(", ")} — the next review knows about these.</p>` : "")
  }

  noteHTML(n) {
    const st = this.s.style
    const sel = st.selected === n.id
    const lost = n.quote !== "" && this.noteRange(n) === false
    let acts = ""

    if (st.keeping === n.id) {
      acts = `<div class="pr-keep"><label for="pr-keep-reason">Why keep it? (optional — the next review reads this)</label>
        <textarea id="pr-keep-reason" rows="2" data-key="keep" class="pr-input"></textarea>
        <div class="pr-acts">${btn("keepSave", n.id, "Keep", "btn btn-primary")}${btn("cancel", n.id, "Cancel")}</div></div>`
    } else if (sel || lost) {
      acts = `<div class="pr-acts">${btn("done", n.id, "Mark done", "btn btn-primary")}${btn("keep", n.id, "Keep as is")}</div>`
    }

    return `<div class="pr-card${sel ? " is-selected" : ""}${lost ? " is-lost" : ""}" data-kind="${n.kind}">
      <button type="button" class="pr-card-main" data-act="note" data-id="${esc(n.id)}" aria-expanded="${sel}">
        <span class="pr-kind"><span class="pr-dot"></span>${n.kind === "other" ? "Style" : cap(n.kind)}<span class="pr-badge">${n.continues_number > 0 ? `Still open from review ${n.continues_number}` : "New"}</span></span>
        <span class="pr-text">${esc(n.text)}</span>
      </button>
      ${lost ? `<div class="pr-lost"><p>${LOST_TEXT}</p><span class="pr-quote">«${esc(n.quote)}»</span></div>` : ""}${acts}
    </div>`
  }

  footHTML() {
    const { review, status } = this.s.style
    const r = this.s.remaining

    if (!review) return ""

    return `<div class="pr-foot"><label for="pr-next-note">Note for the next review (optional)</label>
      <textarea id="pr-next-note" rows="2" data-key="next" class="pr-input"></textarea>
      <div class="pr-acts">${btn("retryStyle", "", "Review again", "btn btn-primary", status === "loading" || (r && r.style <= 0) ? "disabled" : "")}${btn("fresh", "", "Start fresh", "pr-link")}</div></div>`
  }
}
