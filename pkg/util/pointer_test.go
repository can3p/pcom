package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPointer(t *testing.T) {
	t.Parallel()

	t.Run("pointer to int", func(t *testing.T) {
		val := 42
		ptr := Pointer(val)
		require.NotNil(t, ptr)
		require.Equal(t, val, *ptr)
		require.Equal(t, &val, ptr)
	})

	t.Run("pointer to string", func(t *testing.T) {
		val := "hello"
		ptr := Pointer(val)
		require.NotNil(t, ptr)
		require.Equal(t, val, *ptr)
		require.Equal(t, &val, ptr)
	})

	t.Run("pointer to bool", func(t *testing.T) {
		val := true
		ptr := Pointer(val)
		require.NotNil(t, ptr)
		require.Equal(t, val, *ptr)
		require.True(t, *ptr)
	})

	t.Run("pointer to zero value", func(t *testing.T) {
		val := 0
		ptr := Pointer(val)
		require.NotNil(t, ptr)
		require.Equal(t, val, *ptr)
		require.Equal(t, 0, *ptr)
	})

	t.Run("pointer to empty string", func(t *testing.T) {
		val := ""
		ptr := Pointer(val)
		require.NotNil(t, ptr)
		require.Equal(t, val, *ptr)
		require.Equal(t, "", *ptr)
	})

	t.Run("pointer to struct", func(t *testing.T) {
		type TestStruct struct {
			Name  string
			Value int
		}
		val := TestStruct{Name: "test", Value: 123}
		ptr := Pointer(val)
		require.NotNil(t, ptr)
		require.Equal(t, val, *ptr)
		require.Equal(t, "test", ptr.Name)
		require.Equal(t, 123, ptr.Value)
	})

	t.Run("pointer to slice", func(t *testing.T) {
		val := []int{1, 2, 3}
		ptr := Pointer(val)
		require.NotNil(t, ptr)
		require.Equal(t, val, *ptr)
		require.Len(t, *ptr, 3)
	})

	t.Run("pointer to map", func(t *testing.T) {
		val := map[string]int{"a": 1, "b": 2}
		ptr := Pointer(val)
		require.NotNil(t, ptr)
		require.Equal(t, val, *ptr)
		require.Equal(t, 1, (*ptr)["a"])
	})

	t.Run("multiple calls create new pointers", func(t *testing.T) {
		val := 42
		ptr1 := Pointer(val)
		ptr2 := Pointer(val)
		// Even though they point to copies of the same value,
		// they should be different pointers
		require.NotSame(t, ptr1, ptr2)
		require.Equal(t, *ptr1, *ptr2)
	})
}
