import { Controller } from "@hotwired/stimulus"

let galleryCounter = 0

const reduceMotion = () =>
    window.matchMedia("(prefers-reduced-motion: reduce)").matches

// scrollend is the reliable "the user stopped swiping" signal, but it only
// reached every engine at the end of 2025, so fall back to a debounced
// scroll listener on older browsers.
const hasScrollEnd = typeof window !== "undefined" && "onscrollend" in window

// Thin wrapper over a scroll-snap container. The browser does the swiping,
// the momentum and the snapping - we only need to know which slide is
// showing and be able to jump to a given one.
function snapScroller(track, onChange) {
    const index = () => {
        const width = track.clientWidth

        return width > 0 ? Math.round(track.scrollLeft / width) : 0
    }

    const goTo = (i, smooth = true) => {
        const clamped = Math.max(0, Math.min(i, track.children.length - 1))

        track.scrollTo({
            left: clamped * track.clientWidth,
            behavior: smooth && !reduceMotion() ? "smooth" : "auto",
        })
    }

    let timer = null
    const handler = () => {
        if (hasScrollEnd) {
            onChange(index())
            return
        }

        clearTimeout(timer)
        timer = setTimeout(() => onChange(index()), 100)
    }

    track.addEventListener(hasScrollEnd ? "scrollend" : "scroll", handler, {
        passive: true,
    })

    const destroy = () => {
        clearTimeout(timer)
        track.removeEventListener(
            hasScrollEnd ? "scrollend" : "scroll",
            handler
        )
    }

    return { index, goTo, destroy }
}

// Split the rendered markdown into slides.
//
// The markdown inside a {gallery} block comes out as a flat list of
// paragraphs. Every image starts a new slide, and every paragraph that
// follows it - until the next image - is that slide's caption. Paragraphs
// before the first image are not captions of anything, so they get lifted
// out and shown above the gallery.
function collectSlides(inner, leading) {
    const slides = []
    let current = null

    // this is a live html collection: taking a node out of it mutates the
    // collection, so always look at the first remaining element rather than
    // iterating with an index
    const paragraphs = inner.children

    while (paragraphs.length > 0) {
        const p = paragraphs.item(0)
        const imgs = p.querySelectorAll("img")

        if (imgs.length === 0) {
            if (current) {
                current.caption.push(p)
                inner.removeChild(p)
            } else {
                // appendChild moves the node, which also drops it from inner
                leading.appendChild(p)
            }

            continue
        }

        // a single paragraph can hold several images when they are not
        // separated by blank lines in markdown. Each becomes its own slide,
        // and a caption that follows attaches to the last of them.
        for (const img of imgs) {
            current = {
                img,
                full: img.dataset.full || img.src,
                alt: img.alt,
                caption: [],
            }
            slides.push(current)
        }

        inner.removeChild(p)
    }

    return slides
}

function button(className, label, glyph) {
    const el = document.createElement("button")
    el.type = "button"
    el.className = className
    el.setAttribute("aria-label", label)
    el.innerHTML = `<i class="bi bi-${glyph}" aria-hidden="true"></i>`
    return el
}

function counter(className) {
    const el = document.createElement("div")
    el.className = className
    // announce slide changes to screen readers without stealing focus
    el.setAttribute("aria-live", "polite")
    el.setAttribute("aria-atomic", "true")
    return el
}

// The strip shown inline in the post.
function buildStrip(slides, galleryId) {
    const gallery = document.createElement("div")
    gallery.className = "gallery"
    gallery.id = galleryId

    const track = document.createElement("div")
    track.className = "gallery__track"

    slides.forEach((slide, index) => {
        const figure = document.createElement("figure")
        figure.className = "gallery__item"

        const link = document.createElement("a")
        link.className = "gallery__link"
        // stays a working link to the full size image if our js never runs
        link.href = slide.full
        link.dataset.index = String(index)

        slide.img.className = "gallery__img"
        slide.img.loading = "lazy"
        link.appendChild(slide.img)
        figure.appendChild(link)

        if (slide.caption.length > 0) {
            const caption = document.createElement("figcaption")
            caption.className = "gallery__caption"

            for (const p of slide.caption) {
                caption.appendChild(p)
            }

            figure.appendChild(caption)
            slide.captionHTML = caption.innerHTML
        }

        track.appendChild(figure)
    })

    gallery.appendChild(track)

    let prev = null
    let next = null
    let count = null

    if (slides.length > 1) {
        const bar = document.createElement("div")
        bar.className = "gallery__bar"
        prev = button("gallery__nav", "Previous image", "chevron-left")
        next = button("gallery__nav", "Next image", "chevron-right")
        count = counter("gallery__counter")
        bar.append(prev, count, next)
        gallery.appendChild(bar)
    }

    return { gallery, track, prev, next, count }
}

// The fullscreen viewer. A <dialog> gives us the top layer, the backdrop,
// focus trapping and escape-to-close for free; the track inside it is the
// same scroll-snap pattern as the inline strip.
function buildViewer(slides) {
    const dialog = document.createElement("dialog")
    dialog.className = "viewer"
    dialog.setAttribute("aria-label", "Photo gallery")

    const track = document.createElement("div")
    track.className = "viewer__track"

    slides.forEach((slide) => {
        const figure = document.createElement("figure")
        figure.className = "viewer__item"

        const img = document.createElement("img")
        img.className = "viewer__img"
        img.alt = slide.alt
        // src is filled in on demand so opening the viewer does not pull
        // every full size image at once
        img.dataset.src = slide.full
        figure.appendChild(img)

        if (slide.captionHTML) {
            const caption = document.createElement("figcaption")
            caption.className = "viewer__caption"
            caption.innerHTML = slide.captionHTML
            figure.appendChild(caption)
        }

        track.appendChild(figure)
    })

    dialog.appendChild(track)
    dialog.appendChild(button("viewer__close", "Close", "x-lg"))

    let prev = null
    let next = null
    let count = null

    if (slides.length > 1) {
        prev = button("viewer__nav viewer__nav--prev", "Previous image", "chevron-left")
        next = button("viewer__nav viewer__nav--next", "Next image", "chevron-right")
        count = counter("viewer__counter")
        dialog.append(prev, next, count)
    }

    return { dialog, track, prev, next, count }
}

export default class extends Controller {
    connect() {
        this.cachedHTML = this.element.innerHTML

        const inner = this.element.querySelector(
            ".block-container-edit-preview-gallery-content, .block-container-gallery-content"
        )

        if (!inner) {
            return
        }

        const fragment = document.createDocumentFragment()
        const leading = document.createElement("div")
        fragment.appendChild(leading)

        const slides = collectSlides(inner, leading)

        if (slides.length === 0) {
            return
        }

        this.slides = slides
        this.total = slides.length

        const galleryId = `gallery-${++galleryCounter}`
        const strip = buildStrip(slides, galleryId)
        const viewer = buildViewer(slides)

        strip.gallery.appendChild(viewer.dialog)
        fragment.appendChild(strip.gallery)

        this.element.innerHTML = ""
        this.element.appendChild(fragment)

        this.strip = strip
        this.viewer = viewer
        this.listeners = []

        this.stripScroller = snapScroller(strip.track, (i) =>
            this.render(strip, i)
        )
        this.viewerScroller = snapScroller(viewer.track, (i) => {
            this.preload(i)
            this.render(viewer, i)
        })

        this.render(strip, 0)
        this.render(viewer, 0)

        this.wire()
    }

    // one place that knows how to write "3 / 8" and grey out the arrows
    render(ui, index) {
        if (!ui || !ui.count) {
            return
        }

        ui.count.textContent = `${index + 1} / ${this.total}`
        ui.prev.disabled = index === 0
        ui.next.disabled = index === this.total - 1
    }

    // load the current image and its neighbours, leave the rest alone
    preload(index) {
        for (let i = index - 1; i <= index + 1; i++) {
            const figure = this.viewer.track.children[i]

            if (!figure) {
                continue
            }

            const img = figure.querySelector(".viewer__img")

            if (img && !img.src && img.dataset.src) {
                img.src = img.dataset.src
            }
        }
    }

    on(target, event, handler, options) {
        target.addEventListener(event, handler, options)
        this.listeners.push([target, event, handler, options])
    }

    wire() {
        const { strip, viewer } = this

        this.on(strip.track, "click", (event) => {
            const link = event.target.closest(".gallery__link")

            if (!link || event.metaKey || event.ctrlKey || event.shiftKey) {
                return
            }

            event.preventDefault()
            this.open(Number(link.dataset.index))
        })

        if (strip.prev) {
            this.on(strip.prev, "click", () =>
                this.stripScroller.goTo(this.stripScroller.index() - 1)
            )
            this.on(strip.next, "click", () =>
                this.stripScroller.goTo(this.stripScroller.index() + 1)
            )
        }

        this.on(viewer.dialog.querySelector(".viewer__close"), "click", () =>
            viewer.dialog.close()
        )

        if (viewer.prev) {
            this.on(viewer.prev, "click", () =>
                this.viewerScroller.goTo(this.viewerScroller.index() - 1)
            )
            this.on(viewer.next, "click", () =>
                this.viewerScroller.goTo(this.viewerScroller.index() + 1)
            )
        }

        this.on(viewer.dialog, "keydown", (event) => {
            if (event.key === "ArrowLeft") {
                event.preventDefault()
                this.viewerScroller.goTo(this.viewerScroller.index() - 1)
            } else if (event.key === "ArrowRight") {
                event.preventDefault()
                this.viewerScroller.goTo(this.viewerScroller.index() + 1)
            }
        })

        // clicking the backdrop - anything that is not the image itself
        this.on(viewer.track, "click", (event) => {
            if (!event.target.closest(".viewer__img, .viewer__caption")) {
                viewer.dialog.close()
            }
        })

        // escape and the close button both land here
        this.on(viewer.dialog, "close", () => {
            this.stripScroller.goTo(this.viewerScroller.index(), false)
        })
    }

    open(index) {
        const { dialog, track } = this.viewer

        this.preload(index)
        dialog.showModal()

        // the track has no layout until the dialog is open, so the jump to
        // the right slide has to happen after showModal
        track.scrollLeft = index * track.clientWidth
        this.render(this.viewer, index)
    }

    disconnect() {
        if (this.viewer && this.viewer.dialog.open) {
            this.viewer.dialog.close()
        }

        for (const [target, event, handler, options] of this.listeners || []) {
            target.removeEventListener(event, handler, options)
        }

        if (this.stripScroller) {
            this.stripScroller.destroy()
        }

        if (this.viewerScroller) {
            this.viewerScroller.destroy()
        }

        this.listeners = null
        this.strip = null
        this.viewer = null
        this.slides = null
        this.stripScroller = null
        this.viewerScroller = null

        this.element.innerHTML = this.cachedHTML
    }
}
