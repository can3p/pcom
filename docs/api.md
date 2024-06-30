# API

## Fetch posts

```
curl -v -H'Authorization: Bearer 01904236-6811-77fe-8076-f7e80f9a8b99' http://localhost:8080/api/v1/posts | jq .
{
  "data": {
    "posts": [
      {
        "id": "018fa64b-0f9e-7933-b44d-33eae44ccfe1",
        "subject": "",
        "md_body": "# We love headers",
        "visibility": "direct_only",
        "is_published": true,
        "public_url": "http://localhost:8080/posts/018fa64b-0f9e-7933-b44d-33eae44ccfe1"
      },
      ...
    ],
    "cursor": ""
  }
}
```

If cursor is not empty, pass it as `cursor` query parameter with the next call to get next page

## Upload an Image

```
curl -v -H'Authorization: Bearer <api-key>' -XPUT -F 'file=@path/to/image.png' http://localhost:8080/api/v1/image
{"data":{"image_id":"0190478c-5592-74ab-9d1a-5cdab598f2dd.png"}}%
```

## Create new Post

```
curl -v -H'Authorization: Bearer <api-key>' -XPOST -d'{ "subject": "test post", "md_body": "is saved\n\n![trololo](0190478c-5592-74ab-9d1a-5cdab598f2dd.png)", "visibility": "direct_only" }' http://localhost:8080/api/v1/posts
{"data":{"id":"01904796-62f7-7a9a-a7bd-1595ed6d1663","public_url":"http://localhost:8080/posts/01904796-62f7-7a9a-a7bd-1595ed6d1663"}}%
```

## Update a Post

```
curl -v -H'Authorization: Bearer <api-key>' -XPOST -d'{ "subject": "test post1", "md_body": "is **saved**\n\n![trololo](0190478c-5592-74ab-9d1a-5cdab598f2dd.png)", "visibility": "direct_only" }' http://localhost:8080/api/v1/posts/01904796-62f7-7a9a-a7bd-1595ed6d1663
{"data":{"id":"01904796-62f7-7a9a-a7bd-1595ed6d1663","public_url":"http://localhost:8080/posts/01904796-62f7-7a9a-a7bd-1595ed6d1663"}}%
```

## Publish a Post

```
curl -v -H'Authorization: Bearer <api-key>' -XPOST -d'{ "subject": "test post1", "md_body": "is **saved**\n\n![trololo](0190478c-5592-74ab-9d1a-5cdab598f2dd.png)", "visibility": "direct_only", "is_published": true }' http://localhost:8080/api/v1/posts/01904796-62f7-7a9a-a7bd-1595ed6d1663
{"data":{"id":"01904796-62f7-7a9a-a7bd-1595ed6d1663","public_url":"http://localhost:8080/posts/01904796-62f7-7a9a-a7bd-1595ed6d1663"}}%
```

## Delete a Post

```
curl -v -H'Authorization: Bearer <api-key>' -XDELETE http://localhost:8080/api/v1/posts/01904796-62f7-7a9a-a7bd-1595ed6d1663
{"data":null}
```
