Private communities, manifesto
why

Most of the popular platforms are heavily biased in favor of public outreach.
Eventhough that's a good way to create a marketplace and sell ads, this model
results in a very particular user behavior where self promotion and shock
content end up being the most lucrative kind of content. In addition to that
the public nature of the platforms makes users vulnurable to hate storms,
trolling and bots.

The other omnipresent feature of the platforms is a programmatic infinite feed
of updates which boosts pageviews and makes the platform addictive.  The reason
for all this lies in vc funding and corporate structure of the platforms which
requires every growing profits.

The platforms also have no incentive to keep the content forever or surface it
at all.

In the very beginning all social networks started as a way to get updates from
your friends and relatives and to discover people which you like to read and
possible want to get connected offline.

Why don't we return to this idea taking into account some lessons learnt?

## Idea

We can make a stripped down version of blog platform (no communities, no custom
styles) with a focus on privacy of interaction and human touch.

What we want to attempt to get back is every day journals and personal notes.

Here is an outline:

* Human to human.

  Connections are reciprocal (like linkedin), they cannot be made blindly (not
  like linkedin).

  - There is no signup, a user should be invited

    Real users should invite real users. A user should vouch for new invitees
    with their account

  - If there is no common connection, a user should whitelist another one in
    order for them to issue a connection request.

    Random people will not be able to bother you

  - If there is a common connection, a user can ask them to mediate the
    connection request and do an introduction

* Connections as a discovery mechanic

  There is no common feed to read, there is no rating, no kind of global
  discovery. Since a user had been invited by another one, their feed will
  start with the posts from that user exclusively + system journal with news.

  The default feed for a given user will only show the content from their
  direct connections.  The discovery can be done by checking the comments
  (connections of connections) and reading the posts from connections of
  connections in case they allow that (comments should be hidden in this case,
  since they may be from your 3rd level connections).

  It's completely ok to be done with all new posts for the day, no infinite
  feeds.

* Long form

  The focus will be on the longer type of texts. You can do quick short posts
  but the platform should discourage or group that to limit the noise.

  Text content is markdown with images and possibly embeds github style.

  There is no need to keep it that way forever, we new post types may be
  allowed

  Comments are threaded, emotions for posts are available (not for comments).
  It may be possible to comment certain parts of the posts.

* Users in control of their feed

  There is no reason for a user to read all the posts from their connections.
  It's possible that you want to keep the connection, but the posts are never
  interesting or you only like a certain types of updates from a user. A user
  should be able to filter out the posts they do not want

* Completely open

  Since there is no focus on pageviews and monetisation, there is no reason to
  hold to content either, which means

  - Opensource implementation in case somebody wants to launch a separate instance

    This does not mean any sort of federation.

  - Full import/export, in case a user want to get out.
  - Per user API, no way to crawl the graph for mobile apps and other sorts of
    clients. Official client, if there is one, should use the public api.

* Community responsibility

  Every user is responsible for the people they've invited. If you invitee
  starts posting crazy stuff you need to handle that. If a lot of people flag
  the content of your invitee, you need to handle that.

  - A user can revoke the invite and unless another existing user picks up the
    torch, the journal will be suspended.
  - A user that flags/revokes too many invites is penalised. A flag requires
    an explanation.
  - If you simply don't like the content - do not flag it, just mute it

  As a reader you can help others by marking certain types of the content (e.g.
  self promotion, politics etc). You should also flag the content if it crossed
  all limits. Keep in mind, that you should keep this tool for extreme cases
  like porn and violence. If you're just uncomfortable - mute or remove the
  connection.

## Funding

The best way to avoid distortion by vc is to exclude vcs from the ecuation.
The platform should be worked on non-profit basis and any monetisation should
never be a function of pageviews/reads.

One reason to require a lot of funding is to support highly popular users which
have millions of followers and consequently require a lot of resources from
platform. Popular users also skew the platform towards self promotion.

If there is no focus on pageviews, we can also design a platform to be a bad
platform to become an influencer. For example the lack of any sort of analytics
could be a feature.

In case the plarform has no outliers in terms of connections it should be
relatively cheap to run it even on a very low budget - small user payments and
donations.

## Alternatives

There are several alternatives for long form posts that we can consider:

### Standalone blogs

Standalone blog with rss and comments works for some people, since it provides
total control over the presentation and the content, no big corporation has any
even remote rights on the content. The drawback is the lack of social graph and
discovery. It works for many people in terms of building the brand and the
audience, it's a more challenging task to make it work with a group of friends.

The private communities approach is centralised, however non-vc and open
approach should make it less of an issue.

### Fediverse

Fediverse has primary focus on the federation and that's a noble goal to have.
I allows to build separate communitities with different types of rules and
allowed behaviors while providing a way to interact with users from different
instances.

The challenging part of this approach is privacy since it's really hard to
manage content visibility across different instances, the nomadic identity
is even more challenging.

Current manifest proposes to design a solution that does not try to solve
the problem in a perfect way. Different instances can be done, it's possible
to migrate between them with import/export and no comments between instance.
It may be not for everyone and that's fine.

### Substack

Substack is the most recent platform for long form content and arguable a
very successfull one. It's possible to limit the audience and monetise it
in different ways.

It's more bent towards professional journalism which again generates a
certain kind if user dynamics. The idea behind this manifesto works against
any kind of monetisation explisitly since it's purpose is to have private
notes. Some users may want to get paid for their content and there are different
platforms for that already.

## That's it!
