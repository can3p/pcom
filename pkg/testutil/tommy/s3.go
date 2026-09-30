package tommy

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// ObjectInfo is what tommy recorded of a stored object.
type ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
}

// S3Object returns the object stored under key in Bucket; it fails the test
// if there is none.
func (tm *Tommy) S3Object(t testing.TB, key string) ObjectInfo {
	t.Helper()

	var got struct {
		Key     string `json:"key"`
		Size    int64  `json:"size"`
		Headers struct {
			ContentType string `json:"content_type"`
		} `json:"headers"`
	}

	getJSON(t, tm.APIURL+"/s3/buckets/"+Bucket+"/objects/"+url.PathEscape(key), &got)

	return ObjectInfo{Key: got.Key, Size: got.Size, ContentType: got.Headers.ContentType}
}

// S3Objects lists the objects in Bucket whose keys start with prefix (all of
// them for an empty prefix).
func (tm *Tommy) S3Objects(t testing.TB, prefix string) []ObjectInfo {
	t.Helper()

	var got struct {
		Objects []struct {
			Key     string `json:"key"`
			Size    int64  `json:"size"`
			Headers struct {
				ContentType string `json:"content_type"`
			} `json:"headers"`
		} `json:"objects"`
		IsTruncated bool `json:"is_truncated"`
	}

	getJSON(t, tm.APIURL+"/s3/buckets/"+Bucket+"/objects?prefix="+url.QueryEscape(prefix), &got)
	require.False(t, got.IsTruncated, "tommy: more than 1000 objects under %q", prefix)

	objects := make([]ObjectInfo, 0, len(got.Objects))
	for _, o := range got.Objects {
		objects = append(objects, ObjectInfo{Key: o.Key, Size: o.Size, ContentType: o.Headers.ContentType})
	}

	return objects
}

// S3Event is one recorded S3 request.
type S3Event struct {
	Key string
	// Headers are the headers of the request the app sent.
	Headers http.Header
}

// S3Events returns the recorded S3 events of eventType ("s3.object.put").
// tommy keeps a bounded number of events, so a test picks its own object's
// event by Key soon after the request.
func (tm *Tommy) S3Events(t testing.TB, eventType string) []S3Event {
	t.Helper()

	var got []struct {
		Payload struct {
			Key string `json:"key"`
		} `json:"payload"`
		Raw struct {
			Headers http.Header `json:"headers"`
		} `json:"raw"`
	}

	getJSON(t, tm.APIURL+"/events?plugin=s3&type="+url.QueryEscape(eventType), &got)

	events := make([]S3Event, 0, len(got))
	for _, e := range got {
		events = append(events, S3Event{Key: e.Payload.Key, Headers: e.Raw.Headers})
	}

	return events
}

func getJSON(t testing.TB, u string, into any) {
	t.Helper()

	resp, err := http.Get(u)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode, "GET %s", u)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(into))
}
