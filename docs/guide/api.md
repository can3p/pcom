# API and blg

You can create, edit, publish, list and delete your posts, and upload images, over an HTTP API with your personal API key. blg is the official command-line client built on it.

## Getting a key

Open [Settings](settings.md) and generate an API key. Send it as a bearer token in the `Authorization` header of every request.

## blg

[blg](https://github.com/can3p/blg) is a command-line client for pcom. It lets you write posts in local markdown files with images and publish them from your terminal. Its own repository describes installation and commands.

## The API

The API works with the same posts, visibilities and drafts as the web editor: list your posts, upload an image, create a post, update it, publish it or delete it. Requests and responses are in [the API reference](../api.md).

## Developing against pcom

To try the API against a local copy, see [Running pcom locally](../running.md); the seed data includes a ready API key.
