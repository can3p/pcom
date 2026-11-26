import { Controller } from "@hotwired/stimulus"
import { bootstrapTextareaMarkdown } from "@can3p/headless-mde/headless";

export default class extends Controller {
  static values = {
    upload: String,
  }

  connect() {
    let textarea = this.element.querySelector("textarea")
    // the blockquote extension keeps the `>` markers in place on enter, backspace and paste inside a quote
    const { trigger, dispose, cursor } = bootstrapTextareaMarkdown(textarea, {
      options: { enableBlockquoteExtension: true },
    });

    this.dispose = dispose
    this.textarea = textarea
    this.element.classList.add("mdeditor")

    {
      const div = document.createElement("div")
      div.innerHTML = `<div class="mdeditor__loading">Media is being uploaded</div>`

      const loader = div.firstChild
      this.element.prepend(loader);
    }

    // emulate disabled state during image upload
    textarea.addEventListener("keydown", (e) => {
      if (this.element.classList.contains("mdeditor--loading")) {
        e.preventDefault()
      }
    }, false)

    let insertTag = (tag) => {
      let placeholder

      switch (tag) {
        case "gallery":
          placeholder = "Add a list of images there"
          break;
        case "spoiler":
          placeholder = "This text will be collapsed by default"
          break;
        case "cut":
          placeholder = "This text won't be visible in the feed but will be shown on the full past page"
          break;
        default:
          trigger(cmd)
      }

      this.wrapBlock(tag, placeholder)
    }

    let runCmd = function(cmd, e) {
      e.preventDefault()
      textarea.focus()

      switch (cmd) {
        case "gallery":
        case "spoiler":
        case "cut":
          insertTag(cmd)
          break;
        default:
          trigger(cmd)
      }

      htmx.trigger(textarea, "change")
    }

    if (this.uploadValue) {
      let upload = this.element.querySelector("input[type=file]")

      const uploadFile = async(file) => {
        const formData = new FormData();

        let headers = { }


        let addHeadersStr = document.body.getAttribute("hx-headers")

        if (addHeadersStr) {
          try {
            let parsed = JSON.parse(addHeadersStr)
            headers = { ...headers, ...parsed}
          } catch(e) {
            console.warn("failed to parse additional headers", e)
          }
        }

        formData.append('file', file);

        try {
        let response = await fetch(this.uploadValue, {
            method: 'POST',
            headers: headers,
            body: formData
          })

          if (!response.ok) {
            let respBody = await response.json();
            throw respBody;
          }

          let j = await response.json()
          return j.uploaded_url;
        } catch (e) {
          htmx.trigger('body', "operation:error", e)
        }
      }

      // A line typed right under an image is its caption, so after an upload
      // the caret goes to a new line under the image, unless the author has
      // moved on to another field meanwhile.
      const placeCaretForCaption = (image) => {
        if (!image || document.activeElement !== textarea) {
          return
        }

        const start = textarea.value.indexOf(image);
        if (start === -1) {
          return
        }

        const end = start + image.length;
        if (end < textarea.value.length && textarea.value[end] !== '\n') {
          return
        }

        textarea.setSelectionRange(end, end);
        cursor.insert('\n');
      }

      const uploadFiles = (files) => {
        textarea.focus();
        this.element.classList.add("mdeditor--loading")

        let promises = [];

        for (let file of files) {
          if (!file.type.startsWith('image/')){ continue }

          if (cursor.position.line.text) {
            cursor.insert('\n'); // wrap to next line if some line is not empty
          }

          const loadingPlaceholder = `[uploading (${file.name})...${Math.random()}]`;
          cursor.insertAndScrollIntoView(loadingPlaceholder);

          // the alt text is left for the author: a file name tells a screen
          // reader nothing
          let prom = uploadFile(file).then((url) => {
            const image = `![](${url})`;
            cursor.replace(loadingPlaceholder, image);
            return image;
          }).catch((e) => {
            console.log("image upload failure:" , e)
          });

          promises.push(prom)
        }

        Promise.all(promises).then((images) => {
          // the last image is the last one in the text as well
          placeCaretForCaption(images[images.length - 1]);
          htmx.trigger(textarea, "change")
          upload.value = null;
          this.element.classList.remove("mdeditor--loading")
        })
      }

      const handler = async () => {
        uploadFiles(upload.files ?? [])
      };

      upload.addEventListener('change', handler, false)

      for (let btn of this.element.querySelectorAll("[data-upload]")) {
        btn.addEventListener("click", () => upload.click(), false)
      }

      this.element.addEventListener('dragenter', function(e) {
        e.stopPropagation()
        e.preventDefault()
      }, false)

      this.element.addEventListener('dragover', function(e) {
        e.stopPropagation()
        e.preventDefault()
      }, false)

      this.element.addEventListener('drop', function(e) {
        e.stopPropagation()
        e.preventDefault()

        const dt = e.dataTransfer
        const files = dt.files

        uploadFiles(files)
      }, false)

      textarea.addEventListener('paste', (e) => {
        uploadFiles(e.clipboardData.files)
      }, false)
    }


    for (let btn of this.element.querySelectorAll("[data-command]")) {
      let cmd = btn.dataset.command

      btn.addEventListener("click", runCmd.bind(null, cmd), false)
    }

    let showPreview = this.element.querySelector("#show_preview");

    if (showPreview) {
      htmx.on("draft_saved", (ev) => {
        showPreview.hidden = false;
        showPreview.href = ev.detail.url
      })
    }
  }

  // Puts text into the post as a block of its own, at the cursor (replacing
  // the selection), and lets the form autosave it.
  insertText(text) {
    const ta = this.textarea
    const start = ta.selectionStart
    const end = ta.selectionEnd
    const before = ta.value.slice(0, start)
    const after = ta.value.slice(end)

    const lead = blankLineGap(before, false)
    const trail = after === "" ? "\n" : blankLineGap(after, true)

    ta.setRangeText(lead + text + trail, start, end, "end")
    ta.focus()
    window.htmx.trigger(ta, "change")
  }

  // Wraps the selection (or a placeholder) in {tag}...{/tag} as a block of its
  // own: a blank line separates it from the text around it, but no line is
  // added where one is already there.
  wrapBlock(tag, placeholder) {
    const ta = this.textarea
    let start = ta.selectionStart
    let end = ta.selectionEnd

    // surrounding line breaks are not part of the wrapped text
    while (end > start && ta.value[end - 1] === "\n") { end-- }
    while (start < end && ta.value[start] === "\n") { start++ }

    const before = ta.value.slice(0, start)
    const after = ta.value.slice(end)
    const inner = ta.value.slice(start, end) || placeholder

    const open = `${blankLineGap(before, false)}{${tag}}\n`
    const close = `\n{/${tag}}${after === "" ? "\n" : blankLineGap(after, true)}`

    ta.setRangeText(open + inner + close, start, end, "preserve")
    ta.setSelectionRange(start + open.length, start + open.length + inner.length)
    ta.focus()
  }

  disconnect() {
    this.dispose()
  }
}

// Newlines to add between s and a block next to it, so that a blank line
// separates them; none when s is empty or already ends (starts) with one.
function blankLineGap(s, atStart) {
  if (s === "") { return "" }
  const nl = atStart ? s.match(/^\n*/)[0].length : s.match(/\n*$/)[0].length
  return "\n".repeat(Math.max(0, 2 - nl))
}
