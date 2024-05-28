package types

type Replacer[A any] func(in A) (bool, A)
