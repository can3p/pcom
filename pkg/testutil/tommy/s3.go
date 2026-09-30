package tommy

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

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

	o, ok := tm.lookupS3Object(t, key)
	require.True(t, ok, "tommy: no object %s in %s", key, Bucket)

	return o
}

// WaitS3Object waits up to 5s for an object to be stored under key, for
// objects the app writes in the background, and returns it; it fails the
// test if none appears.
func (tm *Tommy) WaitS3Object(t testing.TB, key string) ObjectInfo {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for {
		if o, ok := tm.lookupS3Object(t, key); ok {
			return o
		}

		if time.Now().After(deadline) {
			t.Fatalf("tommy: no object %s in %s within 5s", key, Bucket)
		}

		time.Sleep(pollEvery)
	}
}

func (tm *Tommy) lookupS3Object(t testing.TB, key string) (ObjectInfo, bool) {
	t.Helper()

	var got struct {
		Key     string `json:"key"`
		Size    int64  `json:"size"`
		Headers struct {
			ContentType string `json:"content_type"`
		} `json:"headers"`
	}

	if !fetchJSON(t, tm.APIURL+"/s3/buckets/"+Bucket+"/objects/"+url.PathEscape(key), &got) {
		return ObjectInfo{}, false
	}

	return ObjectInfo{Key: got.Key, Size: got.Size, ContentType: got.Headers.ContentType}, true
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

// client reads tommy's API; the timeout keeps a stuck container from hanging
// the test binary.
var client = &http.Client{Timeout: 10 * time.Second}

func getJSON(t testing.TB, u string, into any) {
	t.Helper()

	require.True(t, fetchJSON(t, u, into), "GET %s: not found", u)
}

// fetchJSON decodes the response to GET u into into and reports true, or
// reports false on 404; any other status fails the test.
func fetchJSON(t testing.TB, u string, into any) bool {
	t.Helper()

	resp, err := client.Get(u)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return false
	}

	require.Equal(t, http.StatusOK, resp.StatusCode, "GET %s", u)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(into))

	return true
}
