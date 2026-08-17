package gofret

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
)

// state carries everything one top-level conversion needs. A new one is made
// per call, so a Codec stays safe for concurrent use.
type state struct {
	c    *Codec
	errs errorList
	meta *Metadata

	// weak mirrors the codec setting but can be turned on for the duration of
	// a single field carrying the `string` tag option.
	weak bool

	// path is the stack of segments leading to the value being converted.
	// Segments are kept structured and rendered only when a string is
	// actually needed, which keeps the error-free path free of allocations.
	path []pathSeg
}

type pathSeg struct {
	name  string
	idx   int
	isIdx bool
}

func (s *state) push(name string) { s.path = append(s.path, pathSeg{name: name}) }

func (s *state) pushIndex(i int) { s.path = append(s.path, pathSeg{idx: i, isIdx: true}) }

func (s *state) pop() { s.path = s.path[:len(s.path)-1] }

// pathString renders the current location, for example "hosts[2].port".
func (s *state) pathString() string {
	if len(s.path) == 0 {
		return ""
	}

	var sb strings.Builder

	for _, seg := range s.path {
		if seg.isIdx {
			sb.WriteByte('[')
			sb.WriteString(strconv.Itoa(seg.idx))
			sb.WriteByte(']')

			continue
		}

		if sb.Len() > 0 {
			sb.WriteByte('.')
		}

		sb.WriteString(seg.name)
	}

	return sb.String()
}

// fail records an error at the current path and reports whether conversion
// should stop.
func (s *state) fail(src, dst reflect.Value, err error) bool {
	var from reflect.Type
	if src.IsValid() {
		from = src.Type()
	}

	var to reflect.Type
	if dst.IsValid() {
		to = dst.Type()
	}

	return s.errs.add(newError(s.pathString(), from, to, err))
}

// convert writes src into dst, which must be settable.
//
// This is the single entry point for every conversion; there is no separate
// encode and decode path. What happens is decided by the kind of dst, so
// struct-to-map, map-to-struct and struct-to-struct all flow through here and
// share the same tag handling, hooks and type cache.
func (s *state) convert(src, dst reflect.Value, tag Tag) {
	src = unwrapIface(src)

	if !src.IsValid() {
		// An absent source leaves the destination alone so that partially
		// specified input merges into defaults, unless zeroing is requested.
		if s.c.cfg.zeroFields && dst.CanSet() {
			dst.Set(reflect.Zero(dst.Type()))
		}

		return
	}

	src, handled, stop := s.applyHooks(src, dst, tag)
	if stop {
		return
	}

	if !handled {
		var ok bool
		if src, ok = s.applyValueDecoder(src, dst); ok {
			return
		}

		src = s.applyValueEncoder(src)
		src = unwrapIface(src)

		if !src.IsValid() {
			return
		}
	}

	// A destination that knows how to read itself from text is honoured, the
	// mirror of rendering it through encoding.TextMarshaler on the way out.
	if src.Kind() == reflect.String && dst.Kind() != reflect.String && dst.Kind() != reflect.Interface {
		if ok, err := unmarshalText(dst, src.String()); ok {
			if err != nil {
				s.fail(src, dst, err)
			}

			return
		}
	}

	s.dispatch(src, dst, tag)
}

func (s *state) dispatch(src, dst reflect.Value, tag Tag) {
	// A pointer source is followed unless the destination can hold it: a
	// pointer destination rebuilds it, and an interface destination decides
	// for itself whether to keep the indirection.
	if dst.Kind() != reflect.Pointer && dst.Kind() != reflect.Interface {
		for src.Kind() == reflect.Pointer {
			if src.IsNil() {
				if s.c.cfg.zeroFields {
					dst.Set(reflect.Zero(dst.Type()))
				}

				return
			}

			src = unwrapIface(src.Elem())

			if !src.IsValid() {
				return
			}
		}
	}

	switch dst.Kind() {
	case reflect.Interface:
		s.toInterface(src, dst, tag)
	case reflect.Pointer:
		s.toPointer(src, dst, tag)
	case reflect.Map:
		s.toMap(src, dst, tag)
	case reflect.Slice:
		s.toSlice(src, dst, tag)
	case reflect.Array:
		s.toArray(src, dst, tag)
	case reflect.Struct:
		s.toStruct(src, dst)
	default:
		// Identical types copy straight across. Containers are excluded above
		// because they merge into an existing destination.
		if src.Type() == dst.Type() {
			dst.Set(src)

			return
		}

		s.toScalar(src, dst)
	}
}

// toScalar handles every non-composite destination.
func (s *state) toScalar(src, dst reflect.Value) {
	switch dst.Kind() {
	case reflect.Bool:
		s.toBool(src, dst)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		s.toInt(src, dst)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		s.toUint(src, dst)
	case reflect.Float32, reflect.Float64:
		s.toFloat(src, dst)
	case reflect.Complex64, reflect.Complex128:
		s.toComplex(src, dst)
	case reflect.String:
		s.toString(src, dst)
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		// Nothing sensible can be synthesised for these, so they are only
		// carried across when the types already line up, which the caller
		// has already ruled out.
		s.fail(src, dst, ErrUnconvertible)
	default:
		s.fail(src, dst, ErrUnsupportedType)
	}
}

// applyHooks offers the value to the configured hooks.
//
// It reports handled when a hook produced a replacement, and stop when a hook
// failed and the error limit says to give up.
func (s *state) applyHooks(src, dst reflect.Value, tag Tag) (out reflect.Value, handled, stop bool) {
	if len(s.c.cfg.hooks) == 0 {
		return src, false, false
	}

	ctx := HookCtx{
		From:  src.Type(),
		To:    dst.Type(),
		Value: src,
		Tag:   tag,
		state: s,
	}

	for _, hook := range s.c.cfg.hooks {
		res, err := hook(ctx)
		if err != nil {
			if errors.Is(err, ErrSkip) {
				continue
			}

			return src, false, s.fail(src, dst, err)
		}

		next := unwrapIface(reflect.ValueOf(res))
		if !next.IsValid() {
			// A hook that yields nil means "the destination gets nothing".
			if s.c.cfg.zeroFields && dst.CanSet() {
				dst.Set(reflect.Zero(dst.Type()))
			}

			return src, false, true
		}

		return next, true, false
	}

	return src, false, false
}

// applyValueDecoder hands the raw source to a destination that knows how to
// read itself. It reports true when the destination took charge.
func (s *state) applyValueDecoder(src, dst reflect.Value) (reflect.Value, bool) {
	if !dst.CanAddr() {
		return src, false
	}

	if !reflect.PointerTo(dst.Type()).Implements(valueDecoderType) {
		return src, false
	}

	dec, ok := dst.Addr().Interface().(ValueDecoder)
	if !ok {
		return src, false
	}

	if err := dec.DecodeValue(src.Interface()); err != nil {
		if errors.Is(err, ErrSkip) {
			return src, false
		}

		s.fail(src, dst, err)
	}

	return src, true
}

// applyValueEncoder asks a source that knows how to write itself for its
// replacement value.
func (s *state) applyValueEncoder(src reflect.Value) reflect.Value {
	enc, ok := asValueEncoder(src)
	if !ok {
		return src
	}

	res, err := enc.EncodeValue()
	if err != nil {
		if errors.Is(err, ErrSkip) {
			return src
		}

		s.errs.add(newError(s.pathString(), src.Type(), nil, err))

		return reflect.Value{}
	}

	return reflect.ValueOf(res)
}

// asValueEncoder finds a ValueEncoder on the value or on a pointer to it.
func asValueEncoder(v reflect.Value) (ValueEncoder, bool) {
	t := v.Type()

	if t.Implements(valueEncoderType) {
		// A nil pointer would panic inside a value receiver, so leave it be.
		if t.Kind() == reflect.Pointer && v.IsNil() {
			return nil, false
		}

		enc, ok := v.Interface().(ValueEncoder)

		return enc, ok
	}

	if !reflect.PointerTo(t).Implements(valueEncoderType) {
		return nil, false
	}

	if v.CanAddr() {
		enc, ok := v.Addr().Interface().(ValueEncoder)

		return enc, ok
	}

	// Not addressable, so work on a copy to reach the pointer method.
	p := reflect.New(t)
	p.Elem().Set(v)

	enc, ok := p.Interface().(ValueEncoder)

	return enc, ok
}

// unwrapIface peels interface wrappers off a value so that dispatch sees the
// dynamic type.
func unwrapIface(v reflect.Value) reflect.Value {
	for v.IsValid() && v.Kind() == reflect.Interface {
		if v.IsNil() {
			return reflect.Value{}
		}

		v = v.Elem()
	}

	return v
}

// isNilLike reports whether v is one of the kinds that can be nil, and is.
func isNilLike(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface,
		reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// isEmptyValue reports whether v is the zero value of its type, which is what
// `omitempty` tests.
func isEmptyValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}

	return v.IsZero()
}
