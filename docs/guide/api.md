# API and blg

You can create, edit, publish, list and delete your posts, and upload images, over an HTTP API with your personal API key. blg is the official command-line client built on it.

## Getting a key

Open [Settings](settings.md) and create an API key: give it a name and choose its access. A full key opens the whole API; a notes-only key can only create and list notes, which is what a phone should hold. A key is shown only once, right after you create it, because pcom stores only a hash of it; copy it then. Send it as a bearer token in the `Authorization` header of every request.

## blg

[blg](https://github.com/can3p/blg) is a command-line client for pcom. It lets you write posts in local markdown files with images and publish them from your terminal. Its own repository describes installation and commands.

## The API

The API works with the same posts, visibilities and drafts as the web editor: list your posts, upload an image, create a post, update it, publish it or delete it. It also creates and lists your [notes](notes.md), including the voice notes and photos a phone sends. Requests and responses are in [the API reference](../api.md).

## Developing against pcom

To try the API against a local copy, see [Running pcom locally](../running.md); the seed data includes a ready API key.
