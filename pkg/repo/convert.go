package repo

// Converting between sqlboiler's core structs and pkg/model, for R5's pass B:
// the repository boundary speaks model while the queries inside are still
// sqlboiler's. Pass C rewrites the queries and D deletes this file.
//
// Fields are matched by name. A null.X field becomes a pointer, and a core
// struct's relation o.R.<Name> becomes the model's field <Name>. No relation
// is renamed: none of the relations pkg/model carries collides with a column
// field. Should one ever need a different name, it goes into relationRenames.

import (
	"fmt"
	"reflect"
)

// relationRenames maps "<core type>.<R field>" to the model's field name,
// for relations whose model field is not named like sqlboiler's R field.
var relationRenames = map[string]string{}

// toModel converts a core struct pointer to a new *M; nil stays nil.
func toModel[M any](src any) *M {
	sv := reflect.ValueOf(src)
	if !sv.IsValid() || sv.IsNil() {
		return nil
	}

	dst := new(M)
	convert(reflect.ValueOf(dst), sv, map[uintptr]reflect.Value{})

	return dst
}

// toModels converts a core slice (core.XSlice) to []*M.
func toModels[M any](src any) []*M {
	sv := reflect.ValueOf(src)
	if !sv.IsValid() || sv.IsNil() {
		return nil
	}

	dst := make([]*M, sv.Len())
	seen := map[uintptr]reflect.Value{}
	for i := range dst {
		d := reflect.New(reflect.TypeFor[M]())
		convert(d, sv.Index(i), seen)
		dst[i] = d.Interface().(*M)
	}

	return dst
}

// toCore converts a model struct pointer to a new *C; nil stays nil.
func toCore[C any](src any) *C {
	return toModel[C](src)
}

// copyInto overwrites the struct dst points at with src's fields, for a
// write that must hand the database's values (defaults, timestamps) back to
// the caller's struct.
func copyInto(dst, src any) {
	convert(reflect.ValueOf(dst), reflect.ValueOf(src), map[uintptr]reflect.Value{})
}

// convert copies src into dst; both are pointers to structs, one a core
// struct and the other a model struct. seen maps source pointers to their
// converted copies, so the back references sqlboiler sets while loading
// relations (post.R.User.R.Posts) don't recurse forever.
func convert(dst, src reflect.Value, seen map[uintptr]reflect.Value) {
	seen[src.Pointer()] = dst

	d, s := dst.Elem(), src.Elem()
	dt := d.Type()

	_, coreDst := dt.FieldByName("R")
	srcR := s.FieldByName("R")

	var dstR reflect.Value
	if coreDst {
		dstR = reflect.New(d.FieldByName("R").Type().Elem())
	}
	hasRels := false

	for i := range dt.NumField() {
		f := dt.Field(i)
		if !f.IsExported() || f.Name == "R" || f.Name == "L" || f.Anonymous {
			continue
		}
		if sf := s.FieldByName(f.Name); sf.IsValid() && !isRelation(f) {
			convertValue(d.Field(i), sf, seen)
			continue
		}
		if !coreDst {
			// A model relation, read from the core struct's R.
			if !srcR.IsValid() || srcR.IsNil() {
				continue
			}
			convertValue(d.Field(i), srcR.Elem().FieldByName(coreRelName(s.Type(), f.Name)), seen)
			continue
		}
		panic(fmt.Sprintf("convert: %s.%s has no counterpart in %s", dt, f.Name, s.Type()))
	}

	if !coreDst {
		return
	}

	// The model's relation fields go into the core struct's R.
	st := s.Type()
	for i := range st.NumField() {
		f := st.Field(i)
		if !isRelation(f) {
			continue
		}
		rf := dstR.Elem().FieldByName(coreRelName(dt, f.Name))
		if !rf.IsValid() {
			panic(fmt.Sprintf("convert: %s.R has no field for %s.%s", dt, st, f.Name))
		}
		if s.Field(i).IsNil() {
			continue
		}
		convertValue(rf, s.Field(i), seen)
		hasRels = true
	}
	if hasRels {
		d.FieldByName("R").Set(dstR)
	} else {
		d.FieldByName("R").Set(reflect.Zero(d.FieldByName("R").Type()))
	}
}

// isRelation reports whether f is a model relation field.
func isRelation(f reflect.StructField) bool {
	tag, ok := f.Tag.Lookup("bun")
	return ok && len(tag) > 4 && tag[:4] == "rel:"
}

// coreRelName is the R field name of the model relation field name on the
// core type coreType.
func coreRelName(coreType reflect.Type, field string) string {
	for k, v := range relationRenames {
		if v == field && len(k) > len(coreType.Name()) && k[:len(coreType.Name())+1] == coreType.Name()+"." {
			return k[len(coreType.Name())+1:]
		}
	}

	return field
}

// convertValue copies one field: same or convertible types (enums), null
// types and pointers, related structs and slices of them.
func convertValue(dst, src reflect.Value, seen map[uintptr]reflect.Value) {
	st, dt := src.Type(), dst.Type()

	switch {
	case st == dt:
		dst.Set(src)
	case isNull(st) && dt.Kind() == reflect.Pointer:
		if !src.FieldByName("Valid").Bool() {
			dst.Set(reflect.Zero(dt))
			return
		}
		p := reflect.New(dt.Elem())
		p.Elem().Set(src.Field(0).Convert(dt.Elem()))
		dst.Set(p)
	case st.Kind() == reflect.Pointer && isNull(dt):
		dst.Set(reflect.Zero(dt))
		if !src.IsNil() {
			dst.Field(0).Set(src.Elem().Convert(dt.Field(0).Type))
			dst.FieldByName("Valid").SetBool(true)
		}
	case st.Kind() == reflect.Pointer && st.Elem().Kind() == reflect.Struct:
		if src.IsNil() {
			dst.Set(reflect.Zero(dt))
			return
		}
		if done, ok := seen[src.Pointer()]; ok {
			dst.Set(done)
			return
		}
		p := reflect.New(dt.Elem())
		convert(p, src, seen)
		dst.Set(p)
	case st.Kind() == reflect.Slice && dt.Kind() == reflect.Slice && st.Elem().Kind() == reflect.Pointer:
		if src.IsNil() {
			dst.Set(reflect.Zero(dt))
			return
		}
		out := reflect.MakeSlice(dt, src.Len(), src.Len())
		for i := range src.Len() {
			convertValue(out.Index(i), src.Index(i), seen)
		}
		dst.Set(out)
	case st.ConvertibleTo(dt):
		dst.Set(src.Convert(dt))
	default:
		panic(fmt.Sprintf("convert: cannot convert %s to %s", st, dt))
	}
}

// isNull reports whether t is a sqlboiler null type: null.String, null.Time
// or a generated NullEnum, a struct whose first field is the value and which
// has a Valid flag.
func isNull(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}
	_, ok := t.FieldByName("Valid")

	return ok
}
