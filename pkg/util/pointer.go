package util

func Pointer[A any](a A) *A {
	return &a
}
