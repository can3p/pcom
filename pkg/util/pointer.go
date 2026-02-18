package util

//go:fix inline
func Pointer[A any](a A) *A {
	return new(a)
}
