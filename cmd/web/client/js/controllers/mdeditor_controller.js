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

    let runCmd = function(cmd, e) {
      e.preventDefault()
      trigger(cmd)
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
            throw new Error(`Response status: ${response.status}`);
          }

          let j = await response.json()
          return j.uploaded_url;
        } catch (e) {
            console.error('Error:', e);
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
          });

          promises.push(prom)
        }

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
  }

  disconnect() {
    this.dispose()
  }
}
