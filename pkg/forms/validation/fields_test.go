package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateUsername(t *testing.T) {
	require.Error(t, ValidateUsername("a"), "too small is error")
	require.Error(t, ValidateUsername("abcdefghijklmnopqrstuvwxyz"), "to big is error")
	require.Error(t, ValidateUsername("_underscore"), "leading underscore not allowed")
	require.Error(t, ValidateUsername("0digit"), "leading digit is not allowed")
	require.Error(t, ValidateUsername("cekj$$%"), "special chars are not allowed")
	require.Error(t, ValidateUsername("under__score"), "multiple underscores are not allowed")
	require.NoError(t, ValidateUsername("abcd"))
	require.NoError(t, ValidateUsername("abcd_def"))
	require.NoError(t, ValidateUsername("abcd_def0_182"))
	require.NoError(t, ValidateUsername("abcdefghijklmnopqrst"))
}

func TestValidateMinMax(t *testing.T) {
	require.Error(t, ValidateMinMax("test val", "a", 2, 5), "too small is error")
	require.Error(t, ValidateMinMax("test val", "abcde", 2, 4), "too big is error")
	require.NoError(t, ValidateMinMax("test val", "ab", 2, 4), "lower edge")
	require.NoError(t, ValidateMinMax("test val", "abcd", 2, 4), "higher edge")
	require.NoError(t, ValidateMinMax("test val", "abc", 2, 4), "in the middle")
}

func TestValidateEnum(t *testing.T) {
	require.Error(t, ValidateEnum("a", []string{"c"}, []string{"c label"}), "outside of set")
	require.NoError(t, ValidateEnum("a", []string{"a", "c"}, []string{"a label", "c label"}), "outside of set")
}
