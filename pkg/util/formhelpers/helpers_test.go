package formhelpers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/can3p/gogo/forms"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/test", nil)
	return ctx, recorder
}

func testFormAction(c *gin.Context, f forms.Form) {
	// dummy form action for testing
}

func TestRetarget(t *testing.T) {
	t.Parallel()

	t.Run("sets HX-Retarget header", func(t *testing.T) {
		ctx, recorder := newTestContext()
		action := Retarget(testFormAction, "#new-target")

		action(ctx, nil)

		require.Equal(t, "#new-target", recorder.Header().Get("HX-Retarget"))
	})

	t.Run("calls wrapped action", func(t *testing.T) {
		ctx, _ := newTestContext()
		called := false
		wrappedAction := func(c *gin.Context, f forms.Form) {
			called = true
		}

		action := Retarget(wrappedAction, ".selector")
		action(ctx, nil)

		require.True(t, called)
	})

	t.Run("with CSS class selector", func(t *testing.T) {
		ctx, recorder := newTestContext()
		action := Retarget(testFormAction, ".my-class")

		action(ctx, nil)

		require.Equal(t, ".my-class", recorder.Header().Get("HX-Retarget"))
	})

	t.Run("with element ID", func(t *testing.T) {
		ctx, recorder := newTestContext()
		action := Retarget(testFormAction, "#myElement")

		action(ctx, nil)

		require.Equal(t, "#myElement", recorder.Header().Get("HX-Retarget"))
	})

	t.Run("with complex selector", func(t *testing.T) {
		ctx, recorder := newTestContext()
		action := Retarget(testFormAction, "div.container > p")

		action(ctx, nil)

		require.Equal(t, "div.container > p", recorder.Header().Get("HX-Retarget"))
	})
}

func TestTrigger(t *testing.T) {
	t.Parallel()

	t.Run("sets HX-Trigger header with JSON", func(t *testing.T) {
		ctx, recorder := newTestContext()
		events := gin.H{"myEvent": gin.H{"detail": "value"}}
		action := Trigger(testFormAction, events)

		action(ctx, nil)

		headerValue := recorder.Header().Get("HX-Trigger")
		require.NotEmpty(t, headerValue)

		// Verify it's valid JSON
		var parsed gin.H
		err := json.Unmarshal([]byte(headerValue), &parsed)
		require.NoError(t, err)
		require.Equal(t, "value", parsed["myEvent"].(map[string]any)["detail"])
	})

	t.Run("calls wrapped action", func(t *testing.T) {
		ctx, _ := newTestContext()
		called := false
		wrappedAction := func(c *gin.Context, f forms.Form) {
			called = true
		}

		action := Trigger(wrappedAction, gin.H{"event": "data"})
		action(ctx, nil)

		require.True(t, called)
	})

	t.Run("with single event", func(t *testing.T) {
		ctx, recorder := newTestContext()
		events := gin.H{"myEvent": "value"}
		action := Trigger(testFormAction, events)

		action(ctx, nil)

		headerValue := recorder.Header().Get("HX-Trigger")
		var parsed map[string]any
		err := json.Unmarshal([]byte(headerValue), &parsed)
		require.NoError(t, err)
		require.Equal(t, "value", parsed["myEvent"])
	})

	t.Run("with multiple events", func(t *testing.T) {
		ctx, recorder := newTestContext()
		events := gin.H{
			"event1": "value1",
			"event2": "value2",
		}
		action := Trigger(testFormAction, events)

		action(ctx, nil)

		headerValue := recorder.Header().Get("HX-Trigger")
		var parsed map[string]any
		err := json.Unmarshal([]byte(headerValue), &parsed)
		require.NoError(t, err)
		require.Equal(t, "value1", parsed["event1"])
		require.Equal(t, "value2", parsed["event2"])
	})

	t.Run("with nested object event", func(t *testing.T) {
		ctx, recorder := newTestContext()
		events := gin.H{"complexEvent": gin.H{"nested": "data"}}
		action := Trigger(testFormAction, events)

		action(ctx, nil)

		headerValue := recorder.Header().Get("HX-Trigger")
		var parsed map[string]any
		err := json.Unmarshal([]byte(headerValue), &parsed)
		require.NoError(t, err)
		nestedObj, ok := parsed["complexEvent"]
		require.True(t, ok)
		nested, ok := nestedObj.(map[string]any)
		require.True(t, ok)
		require.Equal(t, "data", nested["nested"])
	})
}

func TestReplaceHistory(t *testing.T) {
	t.Parallel()

	t.Run("sets HX-Replace-Url header", func(t *testing.T) {
		ctx, recorder := newTestContext()
		action := ReplaceHistory(testFormAction, "/new-url")

		action(ctx, nil)

		require.Equal(t, "/new-url", recorder.Header().Get("HX-Replace-Url"))
	})

	t.Run("calls wrapped action", func(t *testing.T) {
		ctx, _ := newTestContext()
		called := false
		wrappedAction := func(c *gin.Context, f forms.Form) {
			called = true
		}

		action := ReplaceHistory(wrappedAction, "/url")
		action(ctx, nil)

		require.True(t, called)
	})

	t.Run("with full URL", func(t *testing.T) {
		ctx, recorder := newTestContext()
		action := ReplaceHistory(testFormAction, "/path/to/page?param=value")

		action(ctx, nil)

		require.Equal(t, "/path/to/page?param=value", recorder.Header().Get("HX-Replace-Url"))
	})

	t.Run("with hash", func(t *testing.T) {
		ctx, recorder := newTestContext()
		action := ReplaceHistory(testFormAction, "/page#section")

		action(ctx, nil)

		require.Equal(t, "/page#section", recorder.Header().Get("HX-Replace-Url"))
	})
}

func TestNoContent(t *testing.T) {
	t.Parallel()

	t.Run("sets 204 status code", func(t *testing.T) {
		ctx, _ := newTestContext()
		action := NoContent()

		action(ctx, nil)

		require.Equal(t, http.StatusNoContent, ctx.Writer.Status())
	})
}

func TestComposedActions(t *testing.T) {
	t.Parallel()

	t.Run("RetargetThenTrigger", func(t *testing.T) {
		ctx, recorder := newTestContext()
		action := Retarget(
			Trigger(testFormAction, gin.H{"updated": true}),
			"#target",
		)

		action(ctx, nil)

		require.Equal(t, "#target", recorder.Header().Get("HX-Retarget"))
		headerValue := recorder.Header().Get("HX-Trigger")
		var parsed map[string]any
		err := json.Unmarshal([]byte(headerValue), &parsed)
		require.NoError(t, err)
		require.Equal(t, true, parsed["updated"])
	})

	t.Run("ReplaceHistoryAndRetarget", func(t *testing.T) {
		ctx, recorder := newTestContext()
		action := ReplaceHistory(
			Retarget(testFormAction, ".selector"),
			"/new-path",
		)

		action(ctx, nil)

		require.Equal(t, "/new-path", recorder.Header().Get("HX-Replace-Url"))
		require.Equal(t, ".selector", recorder.Header().Get("HX-Retarget"))
	})
}
