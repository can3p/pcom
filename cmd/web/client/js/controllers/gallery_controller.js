import { Controller } from "@hotwired/stimulus"
import Carousel from 'bootstrap/js/dist/carousel';

function generateDocumentFragment(id) {
  let html = `<div id="${id}" class="carousel slide my-2">
  <div class="carousel-inner">
  </div>
  <button class="carousel-control-prev" type="button" data-bs-target="#${id}" data-bs-slide="prev">
    <span class="carousel-control-prev-icon" aria-hidden="true"></span>
    <span class="visually-hidden">Previous</span>
  </button>
  <button class="carousel-control-next" type="button" data-bs-target="#${id}" data-bs-slide="next">
    <span class="carousel-control-next-icon" aria-hidden="true"></span>
    <span class="visually-hidden">Next</span>
  </button>
</div>`

  var tmpl = document.createElement('template');
  tmpl.innerHTML = html;
  return tmpl.content;
}

function addNewItem(inner, fragmentItemsContainer, currentImg, currentCaption, seenFirstItem) {
  // new image means we should create a card out of the previous one and push it into gallery
  let itemContainer = document.createElement("div")
  itemContainer.classList.add("carousel-item")
  if (!seenFirstItem) {
    itemContainer.classList.add("active")
  }

  currentImg.className = "d-block w-auto m-auto img-gallery"
  itemContainer.appendChild(currentImg)

  if (currentCaption.length > 0) {
    let captionContainer = document.createElement("div")
    captionContainer.className = "carousel-caption d-none d-md-block"

    for (let p of currentCaption) {
      captionContainer.appendChild(p)
    }

    itemContainer.appendChild(captionContainer)
  }

  fragmentItemsContainer.appendChild(itemContainer)
}

export default class extends Controller {
  static values = {
  }

  connect() {
    this.initHTML()
    this.carousel = new Carousel(this.element.querySelector(".carousel"))
  }

  initHTML() {
    this.cachedHTML = this.element.innerHTML
    const generatedID = ("gallery-" + Math.random()).replace(".","")
    let fragment = generateDocumentFragment(generatedID)
    let fragmentItemsContainer = fragment.querySelector(".carousel-inner")

    let inner = this.element.querySelector(".block-container-edit-preview-gallery-content, .block-container-gallery-content")
    let paragraphs = inner.children

    let seenFirstItem = false
    let currentImg = null
    let currentCaption = []

    // this is a live html collection,
    // whenever we take the node out
    // it gets modified and an ordinary loop would screw us
    while (paragraphs.length > 0) {
      let p = paragraphs.item(0)
      let img = p.querySelector("img")

      if (!img) {
        // any leading paragraphs without photo cannot be used as captions, hence we're simply
        // pushing htem outside of gallery

        if (!currentImg) {
          fragment.prepend(p)
          continue
        }

        // otherwise all paragraphs following the image form a caption
        currentCaption.push(p)
        inner.removeChild(p)
        continue
      }

      if (!currentImg) {
        currentImg = img
        inner.removeChild(p)
        continue
      }

      addNewItem(inner, fragmentItemsContainer, currentImg, currentCaption, seenFirstItem)
      inner.removeChild(p)
      seenFirstItem = true
      currentImg = img
      currentCaption = []
    }

    if (currentImg) {
      addNewItem(inner, fragmentItemsContainer, currentImg, currentCaption, seenFirstItem)
    }

    this.element.innerHTML = ''
    this.element.appendChild(fragment)
  }

  disconnect() {
    this.element.innerHTML = this.cachedHTML
  }
}
