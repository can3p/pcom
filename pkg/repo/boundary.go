package repo

// The repository boundary speaks pkg/model while the queries inside are still
// sqlboiler's (R5's pass B). Pass C rewrites the queries and D deletes this
// file together with convert.go.

// all converts the result of a sqlboiler All to model structs.
func all[M any](src any, err error) ([]*M, error) {
	return toModels[M](src), err
}

// write runs a sqlboiler write on a core copy of m and copies the result
// back into m, so database defaults and timestamps reach the caller.
func write[C any](m any, fn func(c *C) error) error {
	c := toCore[C](m)
	err := fn(c)
	copyInto(m, c)

	return err
}

// toCoreEnums converts model enum values to sqlboiler's; nil stays nil.
func toCoreEnums[C, M ~string](in []M) []C {
	if in == nil {
		return nil
	}

	out := make([]C, len(in))
	for i, v := range in {
		out[i] = C(v)
	}

	return out
}
