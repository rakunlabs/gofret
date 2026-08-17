package gofret

import (
	"fmt"
	"reflect"
)

var (
	anyType          = reflect.TypeFor[any]()
	stringType       = reflect.TypeFor[string]()
	mapStringAnyType = reflect.TypeFor[map[string]any]()
	sliceAnyType     = reflect.TypeFor[[]any]()
)

// toInterface writes into an interface destination.
//
// For the empty interface there is no target type to steer the conversion, so
// the value is rendered in its generic form: structs become map[string]any
// and containers of them are rebuilt in kind. This is what makes
// To[map[string]any](v) produce a fully converted tree.
func (s *state) toInterface(src, dst reflect.Value, tag Tag) {
	dt := dst.Type()

	if dt.NumMethod() > 0 {
		st := src.Type()

		switch {
		case st.Implements(dt):
			dst.Set(src)
		case src.CanAddr() && reflect.PointerTo(st).Implements(dt):
			dst.Set(src.Addr())
		default:
			s.fail(src, dst, fmt.Errorf("%w: %s does not implement %s", ErrUnconvertible, st, dt))
		}

		return
	}

	if tag.Has(OptKeep) || s.c.cfg.shallow {
		dst.Set(src)

		return
	}

	out, ok := s.generic(src, tag)
	if !ok {
		return
	}

	if !out.IsValid() {
		dst.Set(reflect.Zero(dt))

		return
	}

	dst.Set(out)
}

// generic renders src in the form used for an empty interface destination.
//
// Values that hold nothing to flatten are returned untouched, so []string
// stays a []string and a time.Time stays a time.Time.
func (s *state) generic(src reflect.Value, tag Tag) (reflect.Value, bool) {
	if !s.c.needsGeneric(src.Type()) {
		return src, true
	}

	switch src.Kind() {
	case reflect.Pointer:
		// A pointer to something that needs flattening is followed, so a
		// *User lands in the map as a map[string]any rather than a pointer to
		// one. Pointers to plain values never reach here, because
		// needsGeneric already sent them through untouched.
		if src.IsNil() {
			return reflect.Value{}, true
		}

		return s.generic(src.Elem(), tag)

	case reflect.Struct:
		mv := reflect.New(mapStringAnyType).Elem()

		s.toMap(src, mv, Tag{})

		return mv, true

	case reflect.Map:
		if src.IsNil() {
			return reflect.Value{}, true
		}

		if src.Type().Key().Kind() != reflect.String {
			return src, true
		}

		mv := reflect.New(mapStringAnyType).Elem()

		s.toMap(src, mv, Tag{})

		return mv, true

	case reflect.Slice:
		if src.IsNil() {
			return reflect.Value{}, true
		}

		fallthrough

	case reflect.Array:
		out := reflect.MakeSlice(sliceAnyType, src.Len(), src.Len())

		for i := range src.Len() {
			s.pushIndex(i)
			s.convert(src.Index(i), out.Index(i), Tag{})
			s.pop()
		}

		return out, true

	case reflect.Interface:
		inner := unwrapIface(src)
		if !inner.IsValid() {
			return reflect.Value{}, true
		}

		return s.generic(inner, tag)

	default:
		return src, true
	}
}

// needsGeneric reports whether values of t hold anything that must be
// rewritten when they land in an empty interface. The answer is cached per
// codec because it depends on the struct analysis.
func (c *Codec) needsGeneric(t reflect.Type) bool {
	if v, ok := c.generics.Load(t); ok {
		return v.(bool)
	}

	res := c.needsGenericRec(t, make(map[reflect.Type]bool, 4))

	c.generics.Store(t, res)

	return res
}

func (c *Codec) needsGenericRec(t reflect.Type, seen map[reflect.Type]bool) bool {
	// A type reached twice is part of a cycle; the outer visit already
	// decides the answer, so break here rather than recursing forever.
	if seen[t] {
		return false
	}

	seen[t] = true

	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return c.needsGenericRec(t.Elem(), seen)
	case reflect.Map:
		return t.Key().Kind() == reflect.String || c.needsGenericRec(t.Elem(), seen)
	case reflect.Interface:
		// The dynamic value is unknown, so it has to be inspected at runtime.
		return true
	case reflect.Struct:
		si := c.structInfoOf(t)

		// A struct with no reachable field, such as time.Time, carries no map
		// representation and is better left alone.
		return len(si.Fields) > 0
	default:
		return false
	}
}

// toPointer writes into a pointer destination, allocating as needed.
func (s *state) toPointer(src, dst reflect.Value, tag Tag) {
	if isNilLike(src) {
		if !dst.IsNil() {
			dst.Set(reflect.Zero(dst.Type()))
		}

		return
	}

	if src.Type() == dst.Type() {
		dst.Set(src)

		return
	}

	if dst.IsNil() || s.c.cfg.zeroFields {
		p := reflect.New(dst.Type().Elem())

		s.convert(src, p.Elem(), tag)

		dst.Set(p)

		return
	}

	s.convert(src, dst.Elem(), tag)
}

// toSlice writes into a slice destination.
func (s *state) toSlice(src, dst reflect.Value, tag Tag) {
	elemType := dst.Type().Elem()

	switch src.Kind() {
	case reflect.Slice, reflect.Array:
		// nothing to do, handled below
	case reflect.String:
		switch elemType.Kind() {
		case reflect.Uint8:
			dst.Set(reflect.ValueOf([]byte(src.String())).Convert(dst.Type()))

			return
		case reflect.Int32:
			dst.Set(reflect.ValueOf([]rune(src.String())).Convert(dst.Type()))

			return
		}

		fallthrough
	default:
		if !s.weak {
			s.fail(src, dst, fmt.Errorf("%w: expected an array or slice (WithStrictTypes forbids lifting single values)", ErrUnconvertible))

			return
		}

		// An empty map stands in for an empty slice; anything else is lifted
		// into a one-element slice.
		if src.Kind() == reflect.Map && src.Len() == 0 {
			dst.Set(reflect.MakeSlice(dst.Type(), 0, 0))

			return
		}

		lifted := reflect.MakeSlice(reflect.SliceOf(src.Type()), 1, 1)
		lifted.Index(0).Set(src)

		src = lifted
	}

	if src.Kind() == reflect.Slice && src.IsNil() {
		if !dst.IsNil() {
			dst.Set(reflect.Zero(dst.Type()))
		}

		return
	}

	n := src.Len()

	if src.Type() == dst.Type() && (dst.IsNil() || s.c.cfg.zeroFields) {
		dst.Set(src)

		return
	}

	out := dst

	switch {
	case out.IsNil() || s.c.cfg.zeroFields:
		out = reflect.MakeSlice(dst.Type(), n, n)
	case out.Len() > n:
		out = out.Slice(0, n)
	default:
		for out.Len() < n {
			out = reflect.Append(out, reflect.Zero(elemType))
		}
	}

	for i := range n {
		s.pushIndex(i)
		s.convert(src.Index(i), out.Index(i), tag)
		s.pop()

		if s.errs.stopped {
			break
		}
	}

	dst.Set(out)
}

// toArray writes into a fixed-size array destination.
func (s *state) toArray(src, dst reflect.Value, tag Tag) {
	size := dst.Type().Len()

	switch src.Kind() {
	case reflect.Slice, reflect.Array:
		// nothing to do, handled below
	case reflect.String:
		switch dst.Type().Elem().Kind() {
		case reflect.Uint8:
			src = reflect.ValueOf([]byte(src.String()))
		case reflect.Int32:
			src = reflect.ValueOf([]rune(src.String()))
		default:
			if !s.weak {
				s.fail(src, dst, fmt.Errorf("%w: expected an array or slice", ErrUnconvertible))

				return
			}
		}
	default:
		if !s.weak {
			s.fail(src, dst, fmt.Errorf("%w: expected an array or slice (WithStrictTypes forbids lifting single values)", ErrUnconvertible))

			return
		}

		if src.Kind() == reflect.Map && src.Len() == 0 {
			dst.Set(reflect.Zero(dst.Type()))

			return
		}

		lifted := reflect.MakeSlice(reflect.SliceOf(src.Type()), 1, 1)
		lifted.Index(0).Set(src)

		src = lifted
	}

	if src.Len() > size {
		s.fail(src, dst, fmt.Errorf("%w: %d values do not fit in [%d]%s",
			ErrOverflow, src.Len(), size, dst.Type().Elem()))

		return
	}

	out := reflect.New(dst.Type()).Elem()
	if !s.c.cfg.zeroFields {
		out.Set(dst)
	}

	for i := range src.Len() {
		s.pushIndex(i)
		s.convert(src.Index(i), out.Index(i), tag)
		s.pop()

		if s.errs.stopped {
			break
		}
	}

	dst.Set(out)
}

// toMap writes into a map destination.
func (s *state) toMap(src, dst reflect.Value, tag Tag) {
	switch src.Kind() {
	case reflect.Struct:
		s.mapFromStruct(src, dst)
	case reflect.Map:
		s.mapFromMap(src, dst)
	case reflect.Slice, reflect.Array:
		if !s.weak {
			s.fail(src, dst, fmt.Errorf("%w: expected a map or struct", ErrUnconvertible))

			return
		}

		// A slice of maps is merged into one map, which is how list-shaped
		// configuration formats express a single table.
		s.ensureMap(dst)

		for i := range src.Len() {
			s.pushIndex(i)
			s.convert(src.Index(i), dst, tag)
			s.pop()

			if s.errs.stopped {
				return
			}
		}
	default:
		s.fail(src, dst, fmt.Errorf("%w: expected a map or struct", ErrUnconvertible))
	}
}

// ensureMap makes the destination map usable, honouring WithZeroFields.
func (s *state) ensureMap(dst reflect.Value) {
	if dst.IsNil() || s.c.cfg.zeroFields {
		dst.Set(reflect.MakeMap(dst.Type()))
	}
}

func (s *state) mapFromMap(src, dst reflect.Value) {
	if src.IsNil() {
		if !dst.IsNil() {
			dst.Set(reflect.Zero(dst.Type()))
		}

		return
	}

	s.ensureMap(dst)

	keyType := dst.Type().Key()
	elemType := dst.Type().Elem()

	iter := src.MapRange()
	for iter.Next() {
		name := keyLabel(iter.Key())

		s.push(name)

		key := reflect.New(keyType).Elem()
		s.convert(iter.Key(), key, Tag{})

		elem := reflect.New(elemType).Elem()
		// Merge into whatever is already stored under this key so that
		// partial input layers over defaults.
		if !s.c.cfg.zeroFields {
			if cur := dst.MapIndex(key); cur.IsValid() {
				elem.Set(cur)
			}
		}

		s.convert(iter.Value(), elem, Tag{})

		s.recordKey()

		s.pop()

		if s.errs.stopped {
			return
		}

		dst.SetMapIndex(key, elem)
	}
}

// keyLabel renders a map key for use in a path.
func keyLabel(k reflect.Value) string {
	k = unwrapIface(k)
	if !k.IsValid() {
		return "<nil>"
	}

	if k.Kind() == reflect.String {
		return k.String()
	}

	str, err := stringify(k)
	if err != nil {
		return fmt.Sprint(k.Interface())
	}

	return str
}
