import { Controller } from "@hotwired/stimulus"
import { bootstrapTextareaMarkdown } from "textarea-markdown-editor/dist/bootstrap";

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

export default class extends Controller {
  static values = {
    upload: String,
  }

  connect() {
    let textarea = this.element.querySelector("textarea")
    const { trigger, dispose, cursor } = bootstrapTextareaMarkdown(textarea, {
      options: {
        enableLinkPasteExtension: false,
      }
    });

    this.dispose = dispose
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

      cursor.wrap([`\n{${tag}}\n`, `\n{/${tag}}\n\n`], { placeholder });
    }

    let runCmd = function(cmd, e) {
      e.preventDefault()

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

      const uploadFiles = async(files) => {
        this.element.classList.add("mdeditor--loading")

        let promises = [];

        for (let file of files) {
          if (!file.type.startsWith('image/')){ continue }

          if (cursor.position.line.text) {
            cursor.insert('\n'); // wrap to next line if some line is not empty
          }
          const loadingPlaceholder = `[uploading (${file.name})...${Math.random()}]`;
          cursor.insert('\n' + loadingPlaceholder + '\n');

          let prom =  uploadFile(file).then((resultUrl) => {
            textarea.value = cursor.value.replace(loadingPlaceholder, `![${file.name}](${resultUrl})`)
          }).catch((e) => {
            console.log("image upload failure:" , e)
          });

          promises.push(prom)
        }

        htmx.trigger(textarea, "change")

        await Promise.all(promises)
        upload.value = null;
        this.element.classList.remove("mdeditor--loading")
      }

      const handler = async () => {
        uploadFiles(upload.files ?? [])
      };

      upload.addEventListener('change', handler, false);

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
        showPreview.classList.remove("d-none");
        showPreview.href = ev.detail.url
      })
    }
  }

  disconnect() {
    this.dispose()
  }
}
