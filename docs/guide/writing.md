# Writing

You write posts in markdown, save them as drafts, preview them and publish when they are ready. Each post has its own visibility, and you can hand a single post to anyone with a share link.

![The new post editor with markdown typed into it.](screenshots/editor.png)

## The editor

Open Write in the top bar. A post has an optional subject, an optional URL it is about, and a body in markdown. The toolbar above the body inserts bold and italic text, quotes, lists, code blocks and links. Three buttons are specific to pcom: a cut that hides the rest of the post from the feed, a spoiler block that hides text until it is opened, and a gallery that groups images.

The editor saves as you type, a couple of seconds after you stop, and shows when it last did. A post you have not published stays a draft; your drafts are listed on the Controls page, where you can delete them. If a connection prompted you, the editor shows their question and your post is linked to it.

## Preview

The page icon in the toolbar opens the post as readers will see it, in a new tab. It appears once the post has been saved for the first time.

## Visibility

Below the body you choose who can read the post: direct connections only, their connections as well, or public. You can change it later. [Connections](connections.md) spells out what each choice means.

## Publishing

Publishing makes the post visible according to its visibility and emails your direct connections about it. You can turn a published post back into a draft or delete it. Only you can edit your posts.

## Images

The camera button uploads one or more images and puts them into the post. Images can also be sent through the [API](api.md).

## Share links

On a published post of yours you can create a share link. Anybody who has the link can read that one post, with or without an account. Delete the link and the address stops working.

## Take your posts with you

Settings has an export of all your posts, with their images, as a zip, and an import that adds the posts of such an archive to your blog. Importing does not deduplicate. The details are in [Settings](settings.md).

![A journal: the posts of one user, as a connection sees them.](screenshots/journal.png)

Your journal, at `/users/<username>`, lists your published posts. Each visitor sees the ones their distance from you allows.
