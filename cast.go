package gofret

import (
	"encoding"
	"fmt"
	"math"
	"reflect"
	"strconv"
)

var (
	textMarshalerType   = reflect.TypeFor[encoding.TextMarshaler]()
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
	stringerType        = reflect.TypeFor[fmt.Stringer]()
	errorType           = reflect.TypeFor[error]()
)

// isJSONNumber reports whether t is encoding/json.Number, recognised without
// importing encoding/json so gofret stays free of that dependency.
func isJSONNumber(t reflect.Type) bool {
	return t.Kind() == reflect.String && t.Name() == "Number" && t.PkgPath() == "encoding/json"
}

func (s *state) toBool(src, dst reflect.Value) {
	if src.Kind() == reflect.Bool {
		dst.SetBool(src.Bool())

		return
	}

	if !s.weak {
		s.failStrict(src, dst)

		return
	}

	switch src.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		dst.SetBool(src.Int() != 0)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		dst.SetBool(src.Uint() != 0)
	case reflect.Float32, reflect.Float64:
		dst.SetBool(src.Float() != 0)
	case reflect.String:
		str := src.String()
		if str == "" {
			dst.SetBool(false)

			return
		}

		b, err := strconv.ParseBool(str)
		if err != nil {
			s.fail(src, dst, fmt.Errorf("%w: %w", ErrUnconvertible, err))

			return
		}

		dst.SetBool(b)
	default:
		s.fail(src, dst, ErrUnconvertible)
	}
}

// failStrict reports a conversion only WithStrictTypes declines, naming the
// option so the cause is obvious.
func (s *state) failStrict(src, dst reflect.Value) {
	s.fail(src, dst, fmt.Errorf("%w (WithStrictTypes forbids it)", ErrUnconvertible))
}

func (s *state) toInt(src, dst reflect.Value) {
	weak := s.weak

	var n int64

	switch src.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n = src.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u := src.Uint()
		if u > math.MaxInt64 {
			s.fail(src, dst, ErrOverflow)

			return
		}

		n = int64(u)
	case reflect.Float32, reflect.Float64:
		f := src.Float()
		// A whole number is carried across exactly; anything else would lose
		// information, so it needs weak typing to be accepted.
		if f != math.Trunc(f) && !weak {
			s.fail(src, dst, fmt.Errorf("%w: %v is not a whole number", ErrUnconvertible, f))

			return
		}

		if f < math.MinInt64 || f > math.MaxInt64 {
			s.fail(src, dst, ErrOverflow)

			return
		}

		n = int64(f)
	case reflect.Bool:
		if !weak {
			s.failStrict(src, dst)

			return
		}

		if src.Bool() {
			n = 1
		}
	case reflect.String:
		if !weak && !isJSONNumber(src.Type()) {
			s.failStrict(src, dst)

			return
		}

		str := src.String()
		if str == "" {
			str = "0"
		}

		parsed, err := strconv.ParseInt(str, 0, dst.Type().Bits())
		if err != nil {
			s.fail(src, dst, fmt.Errorf("%w: %w", ErrUnconvertible, err))

			return
		}

		n = parsed
	default:
		s.fail(src, dst, ErrUnconvertible)

		return
	}

	if dst.OverflowInt(n) {
		s.fail(src, dst, ErrOverflow)

		return
	}

	dst.SetInt(n)
}

func (s *state) toUint(src, dst reflect.Value) {
	weak := s.weak

	var n uint64

	switch src.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i := src.Int()
		if i < 0 {
			if !weak {
				s.fail(src, dst, fmt.Errorf("%w: %d is negative", ErrOverflow, i))

				return
			}

			// Weak typing wraps a negative into the destination width, the
			// two's complement result a C-minded caller expects.
			dst.SetUint(uint64(i) & widthMask(dst.Type().Bits()))

			return
		}

		n = uint64(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		n = src.Uint()
	case reflect.Float32, reflect.Float64:
		f := src.Float()
		if f != math.Trunc(f) && !weak {
			s.fail(src, dst, fmt.Errorf("%w: %v is not a whole number", ErrUnconvertible, f))

			return
		}

		if f < 0 {
			if !weak {
				s.fail(src, dst, fmt.Errorf("%w: %v is negative", ErrOverflow, f))

				return
			}

			dst.SetUint(uint64(int64(f)) & widthMask(dst.Type().Bits()))

			return
		}

		if f > math.MaxUint64 {
			s.fail(src, dst, ErrOverflow)

			return
		}

		n = uint64(f)
	case reflect.Bool:
		if !weak {
			s.failStrict(src, dst)

			return
		}

		if src.Bool() {
			n = 1
		}
	case reflect.String:
		if !weak && !isJSONNumber(src.Type()) {
			s.failStrict(src, dst)

			return
		}

		str := src.String()
		if str == "" {
			str = "0"
		}

		parsed, err := strconv.ParseUint(str, 0, dst.Type().Bits())
		if err != nil {
			s.fail(src, dst, fmt.Errorf("%w: %w", ErrUnconvertible, err))

			return
		}

		n = parsed
	default:
		s.fail(src, dst, ErrUnconvertible)

		return
	}

	if dst.OverflowUint(n) {
		s.fail(src, dst, ErrOverflow)

		return
	}

	dst.SetUint(n)
}

// widthMask returns the mask that keeps the low bits of an integer of the
// given width.
func widthMask(bits int) uint64 {
	if bits >= 64 {
		return ^uint64(0)
	}

	return 1<<uint(bits) - 1
}

func (s *state) toFloat(src, dst reflect.Value) {
	weak := s.weak

	var f float64

	switch src.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		f = float64(src.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		f = float64(src.Uint())
	case reflect.Float32, reflect.Float64:
		f = src.Float()
	case reflect.Bool:
		if !weak {
			s.failStrict(src, dst)

			return
		}

		if src.Bool() {
			f = 1
		}
	case reflect.String:
		if !weak && !isJSONNumber(src.Type()) {
			s.failStrict(src, dst)

			return
		}

		str := src.String()
		if str == "" {
			str = "0"
		}

		parsed, err := strconv.ParseFloat(str, dst.Type().Bits())
		if err != nil {
			s.fail(src, dst, fmt.Errorf("%w: %w", ErrUnconvertible, err))

			return
		}

		f = parsed
	default:
		s.fail(src, dst, ErrUnconvertible)

		return
	}

	if dst.OverflowFloat(f) {
		s.fail(src, dst, ErrOverflow)

		return
	}

	dst.SetFloat(f)
}

func (s *state) toComplex(src, dst reflect.Value) {
	switch src.Kind() {
	case reflect.Complex64, reflect.Complex128:
		c := src.Complex()
		if dst.OverflowComplex(c) {
			s.fail(src, dst, ErrOverflow)

			return
		}

		dst.SetComplex(c)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		dst.SetComplex(complex(float64(src.Int()), 0))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		dst.SetComplex(complex(float64(src.Uint()), 0))
	case reflect.Float32, reflect.Float64:
		dst.SetComplex(complex(src.Float(), 0))
	default:
		s.fail(src, dst, ErrUnconvertible)
	}
}

func (s *state) toString(src, dst reflect.Value) {
	if src.Kind() == reflect.String {
		dst.SetString(src.String())

		return
	}

	// A type that renders itself as text is honoured even in strict mode,
	// because the conversion is the one the type itself defines rather than
	// one gofret invented. This covers encoding.TextMarshaler, fmt.Stringer
	// and error, which is what makes a time.Duration field come out as
	// "1h30m0s" without any hook.
	if str, ok, err := selfText(src); ok {
		if err != nil {
			s.fail(src, dst, err)

			return
		}

		dst.SetString(str)

		return
	}

	if !s.weak {
		s.failStrict(src, dst)

		return
	}

	str, err := stringify(src)
	if err != nil {
		s.fail(src, dst, err)

		return
	}

	dst.SetString(str)
}

// selfText renders a value through whatever text form the type defines for
// itself. The boolean reports whether the type defines one at all.
func selfText(v reflect.Value) (string, bool, error) {
	if str, ok, err := marshalText(v); ok {
		return str, true, err
	}

	t := v.Type()

	switch {
	case t.Implements(stringerType):
		if isNilLike(v) {
			return "", true, nil
		}

		return v.Interface().(fmt.Stringer).String(), true, nil

	case t.Implements(errorType):
		if isNilLike(v) {
			return "", true, nil
		}

		return v.Interface().(error).Error(), true, nil

	case reflect.PointerTo(t).Implements(stringerType):
		p := reflect.New(t)
		p.Elem().Set(v)

		return p.Interface().(fmt.Stringer).String(), true, nil

	default:
		return "", false, nil
	}
}

// marshalText renders a value through encoding.TextMarshaler. The boolean
// reports whether the type implements it at all.
func marshalText(v reflect.Value) (string, bool, error) {
	t := v.Type()

	var m encoding.TextMarshaler

	switch {
	case t.Implements(textMarshalerType):
		if t.Kind() == reflect.Pointer && v.IsNil() {
			return "", true, nil
		}

		m, _ = v.Interface().(encoding.TextMarshaler)
	case reflect.PointerTo(t).Implements(textMarshalerType):
		p := reflect.New(t)
		p.Elem().Set(v)

		m, _ = p.Interface().(encoding.TextMarshaler)
	default:
		return "", false, nil
	}

	if m == nil {
		return "", false, nil
	}

	b, err := m.MarshalText()
	if err != nil {
		return "", true, err
	}

	return string(b), true, nil
}

// stringify renders any value as text. It backs both the `string` tag option
// and weakly typed conversions to a string destination.
func stringify(v reflect.Value) (string, error) {
	if !v.IsValid() {
		return "", nil
	}

	if str, ok, err := selfText(v); ok {
		return str, err
	}

	switch v.Kind() {
	case reflect.String:
		return v.String(), nil
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(v.Uint(), 10), nil
	case reflect.Float32:
		return strconv.FormatFloat(v.Float(), 'f', -1, 32), nil
	case reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64), nil
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return string(v.Bytes()), nil
		}

		if v.Type().Elem().Kind() == reflect.Int32 {
			return string(v.Interface().([]rune)), nil
		}
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return "", nil
		}

		return stringify(v.Elem())
	}

	return "", fmt.Errorf("%w: cannot render %s as a string", ErrUnconvertible, v.Type())
}

// unmarshalText fills a destination through encoding.TextUnmarshaler. The
// boolean reports whether the type implements it at all.
func unmarshalText(dst reflect.Value, text string) (bool, error) {
	if !dst.CanAddr() {
		return false, nil
	}

	if !reflect.PointerTo(dst.Type()).Implements(textUnmarshalerType) {
		return false, nil
	}

	u, ok := dst.Addr().Interface().(encoding.TextUnmarshaler)
	if !ok {
		return false, nil
	}

	return true, u.UnmarshalText([]byte(text))
}
