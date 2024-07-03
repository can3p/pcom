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

        return fetch(this.uploadValue, {
            method: 'POST',
            headers: headers,
            body: formData
          })
          .then((response) => response.json())
          .then((response) => response.uploaded_url)
          .catch((error) => {
            console.error('Error:', error);
          });
      }

      const uploadFiles = async(files) => {
        for (let file of files) {
          if (!file.type.startsWith('image/')){ continue }

          if (cursor.position.line.text) {
            cursor.insert('\n'); // wrap to next line if some line is not empty
          }
          const loadingPlaceholder = `[uploading (${file.name})...${+new Date()}]`;
          cursor.insert('\n' + loadingPlaceholder + '\n');

          let resultUrl = await uploadFile(file)

          upload.value = null

          cursor.setValue(
            cursor.value.replace(
              loadingPlaceholder,
              `![${cursor.MARKER}${file.name}${cursor.MARKER}](${resultUrl})`,
            ),
          );
        }
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
