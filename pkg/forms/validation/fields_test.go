package validation

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestValidateUsername(t *testing.T) {
	assert.Error(t, ValidateUsername("a"), "too small is error")
	assert.Error(t, ValidateUsername("abcdefghijklmnopqrstuvwxyz"), "to big is error")
	assert.Error(t, ValidateUsername("_underscore"), "leading underscore not allowed")
	assert.Error(t, ValidateUsername("0digit"), "leading digit is not allowed")
	assert.Error(t, ValidateUsername("cekj$$%"), "special chars are not allowed")
	assert.Error(t, ValidateUsername("under__score"), "multiple underscores are not allowed")
	assert.NoError(t, ValidateUsername("abcd"))
	assert.NoError(t, ValidateUsername("abcd_def"))
	assert.NoError(t, ValidateUsername("abcd_def0_182"))
	assert.NoError(t, ValidateUsername("abcdefghijklmnopqrst"))
}
