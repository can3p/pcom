package admin_test

import (
	"net"
	"os"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/admin"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/stretchr/testify/require"
)

func TestClonedCustomRecovery_WithPanic(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, "POST", "/test", nil)

	result := admin.ClonedCustomRecovery(c, "test panic")

	// Verify the result contains expected components
	require.NotEmpty(t, result)
	require.Contains(t, result, "[Recovery]")
	require.Contains(t, result, "panic recovered")
	require.Contains(t, result, "test panic")
	// Stack frames should be present
	require.Contains(t, result, "0x")
}

func TestClonedCustomRecovery_WithConnectionError(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, "GET", "/test", nil)

	// Create a connection reset error
	connErr := &net.OpError{
		Op:  "write",
		Net: "tcp",
		Err: &os.SyscallError{
			Syscall: "write",
			Err:     net.ErrClosed,
		},
	}

	result := admin.ClonedCustomRecovery(c, connErr)

	// Result should be non-empty and contain request info
	require.NotEmpty(t, result)
	// May or may not have [Recovery] depending on the exact error type
	require.Contains(t, result, "GET")
}

func TestClonedCustomRecovery_HidesCookie(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, "GET", "/test", nil)
	c.Request.Header.Set("Cookie", "secret_session_token=abc123def456")

	result := admin.ClonedCustomRecovery(c, "test panic")

	// Cookie should be hidden
	require.NotContains(t, result, "secret_session_token=abc123def456")
	require.Contains(t, result, "Cookie: <hidden>")
}

func TestClonedCustomRecovery_HidesAuthorization(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, "GET", "/test", nil)
	c.Request.Header.Set("Authorization", "Bearer secret_token_12345")

	result := admin.ClonedCustomRecovery(c, "test panic")

	// Authorization should be hidden
	require.NotContains(t, result, "secret_token_12345")
	require.Contains(t, result, "Authorization: *")
}

func TestClonedCustomRecovery_StackFramesPresent(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, "POST", "/test", nil)

	result := admin.ClonedCustomRecovery(c, "test panic")

	// Should have stack frame information with file:line format
	lines := strings.Split(result, "\n")
	hasStackFrame := false
	for _, line := range lines {
		// Stack frames contain file paths and line numbers like "filename.go:123"
		if strings.Contains(line, ".go:") && strings.Contains(line, "0x") {
			hasStackFrame = true
			break
		}
	}

	require.True(t, hasStackFrame, "Stack frames should be present in output")
}
