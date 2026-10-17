package admin_test

import (
	"net"
	"os"
	"syscall"
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
	// Stack frames are present in "file.go:line (0x...)" form
	require.Contains(t, result, ".go:")
	require.Contains(t, result, "0x")
}

func TestClonedCustomRecovery_WithBrokenPipe(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, "GET", "/test", nil)

	connErr := &net.OpError{
		Op:  "write",
		Net: "tcp",
		Err: &os.SyscallError{
			Syscall: "write",
			Err:     syscall.EPIPE,
		},
	}

	result := admin.ClonedCustomRecovery(c, connErr)

	// A broken connection is logged as the error and the request only
	require.Contains(t, result, "broken pipe")
	require.Contains(t, result, "GET /test")
	require.NotContains(t, result, "[Recovery]")
	require.NotContains(t, result, ".go:")
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
