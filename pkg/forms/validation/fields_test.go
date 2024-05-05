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

func TestValidateMinMax(t *testing.T) {
	assert.Error(t, ValidateMinMax("test val", "a", 2, 5), "too small is error")
	assert.Error(t, ValidateMinMax("test val", "abcde", 2, 4), "too big is error")
	assert.NoError(t, ValidateMinMax("test val", "ab", 2, 4), "lower edge")
	assert.NoError(t, ValidateMinMax("test val", "abcd", 2, 4), "higher edge")
	assert.NoError(t, ValidateMinMax("test val", "abc", 2, 4), "in the middle")
}

func TestValidateEnum(t *testing.T) {
	assert.Error(t, ValidateEnum("a", []string{"c"}, []string{"c label"}), "outside of set")
	assert.NoError(t, ValidateEnum("a", []string{"a", "c"}, []string{"a label", "c label"}), "outside of set")
}
