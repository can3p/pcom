# Connections

Connections decide who sees your posts and who can talk to you. You can invite someone by email, allow a known user to connect, or ask mutual connections to vouch for you, and each post and profile has a visibility that follows from these links.

## Making connections

A connection is mutual. There are three ways to get one.

- **Invite someone.** In [Settings](settings.md) you send an invitation to an email address. When the person accepts and creates an account, you are connected from the start. Invitations are limited: an account created from an invitation gets one of its own, accounts from open signup get none, and more are handed out by the site admin.
- **Allow someone to connect.** On the Controls page you add a username to your whitelist. That user then sees a connect button on your profile and can use it once. It does not open your profile to them: they must already be able to see it, as set by your profile visibility. You can withdraw the permission before they do.
- **Ask for mediation.** On the profile of someone you share a connection with, you can ask your common connections to vouch for you, with a note. Each of them can sign or dismiss the request. Once at least one has signed, the person you asked sees the request with the signatures and approves or dismisses it. Approving creates the connection. You can withdraw a request that is still open.

The Controls page lists your connections, your connections' connections, drafts, whitelist entries and the mediation and connection requests waiting for you. You can drop a connection from the other person's profile. You can also send a connection a prompt, a short question that shows up in their feed and offers to start a post as the answer; prompts are rate limited.

## Who is "second degree"

The connections of your direct connections are your second-degree connections. Anyone further away is a stranger, as is anyone you are not connected to at all.

## Who can see a post

Only published posts are visible to anybody but you. Drafts are yours alone.

| Post visibility | You and direct connections | Second-degree | Everyone else |
|---|---|---|---|
| Direct connections only | yes | no | no |
| Their connections as well | yes | yes | no |
| Public | yes | yes | yes, also without an account |

"Their connections as well" means the connections of the author's direct connections. A post you can't see looks like a post that doesn't exist to a logged-in user; an anonymous visitor is sent to the login page instead.

Only you and your direct connections can read and write comments on a post, whatever its visibility. A second-degree reader or a stranger sees the post without the discussion. See [Comments](comments.md).

## Who can see a profile

Your profile visibility, set in [Settings](settings.md), decides who can open your journal page at all:

- **Public**: anybody, with or without an account.
- **Registered users**: anybody who is logged in.
- **Direct and indirect connections**: your direct and second-degree connections only.

Opening a profile does not open every post in it. On a journal, you and direct connections see all published posts, second-degree connections see the second-degree and public ones, and anonymous visitors see the public ones. A logged-in user with no connection to the author sees "You are not allowed to see posts in this journal"; they can read a public post only at its own address or in Explore. A public post of a profile that is not public is still readable at its own address but is left out of the public index and the public RSS feed. [Feed and RSS](feed.md) covers where public posts are listed.

A share link (see [Writing](writing.md)) is the one exception: it shows a single published post to whoever holds the link.

The rules live in the visibility code of the reading service; [architecture](../architecture.md) describes how that layer is arranged.
