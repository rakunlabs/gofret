package gofret

import (
	"reflect"
)

// HookCtx describes the value a hook is being offered.
//
// The cheap facts are fields; anything that costs an allocation is a method,
// so a hook that only inspects types and declines pays nothing.
type HookCtx struct {
	// From is the type of the source value. It is nil when the source is
	// an untyped nil.
	From reflect.Type
	// To is the type of the destination. When writing into a map[string]any
	// or an any field this is the empty interface, not the concrete type of
	// the source.
	To reflect.Type
	// Value is the source value.
	Value reflect.Value
	// Tag is the parsed struct tag of the field being converted. It is the
	// zero Tag when the value is not a struct field.
	Tag Tag

	state *state
}

// Data returns the source value as an any, so a simple hook needs no
// reflection. It boxes the value, so prefer Value when you are only going to
// inspect it.
func (c HookCtx) Data() any {
	if !c.Value.IsValid() {
		return nil
	}

	return c.Value.Interface()
}

// Path returns the dotted location of the value, for example
// "database.hosts[2].port". It is empty at the root.
//
// The path is built on demand, so asking for it only in an error branch costs
// nothing on the happy path.
func (c HookCtx) Path() string {
	if c.state == nil {
		return ""
	}

	return c.state.pathString()
}

// Hook replaces a value before the built-in conversion runs.
//
// Return ErrSkip to decline, which passes the value to the next hook and
// finally to the built-in conversion. Any other error aborts the conversion
// and is reported to the caller, so a hook can report a genuine failure
// instead of silently declining.
//
// The returned value is converted to the destination as usual, so a hook may
// hand back an intermediate representation rather than the final type.
type Hook func(HookCtx) (any, error)

// HookTo builds a Hook that fires when the destination type is exactly T.
//
//	gofret.HookTo(func(in any) (time.Duration, error) {
//	    s, ok := in.(string)
//	    if !ok {
//	        return 0, gofret.ErrSkip
//	    }
//	    return time.ParseDuration(s)
//	})
func HookTo[T any](fn func(any) (T, error)) Hook {
	to := reflect.TypeFor[T]()

	return func(ctx HookCtx) (any, error) {
		if ctx.To != to {
			return nil, ErrSkip
		}

		return fn(ctx.Data())
	}
}

// HookFrom builds a Hook that fires when the source value is assignable to T,
// whatever the destination is.
//
// This is the form to use when writing into a map, where the destination type
// is the empty interface and so carries no information.
//
//	gofret.HookFrom(func(t time.Time) (any, error) {
//	    return t.Format(time.RFC3339), nil
//	})
func HookFrom[T any](fn func(T) (any, error)) Hook {
	from := reflect.TypeFor[T]()

	return func(ctx HookCtx) (any, error) {
		v, ok := coerce(ctx, from)
		if !ok {
			return nil, ErrSkip
		}

		return fn(v.Interface().(T))
	}
}

// HookBetween builds a Hook that fires when the source is assignable to In
// and the destination type is exactly Out.
//
//	gofret.HookBetween(func(s string) (time.Time, error) {
//	    return time.Parse(time.RFC3339, s)
//	})
func HookBetween[In, Out any](fn func(In) (Out, error)) Hook {
	from := reflect.TypeFor[In]()
	to := reflect.TypeFor[Out]()

	return func(ctx HookCtx) (any, error) {
		if ctx.To != to {
			return nil, ErrSkip
		}

		v, ok := coerce(ctx, from)
		if !ok {
			return nil, ErrSkip
		}

		return fn(v.Interface().(In))
	}
}

// coerce reports whether the source value can be handed to a hook expecting
// the type want, returning it in that exact shape.
func coerce(ctx HookCtx, want reflect.Type) (reflect.Value, bool) {
	if ctx.From == nil || !ctx.Value.IsValid() {
		return reflect.Value{}, false
	}

	if want.Kind() == reflect.Interface {
		if !ctx.From.Implements(want) {
			return reflect.Value{}, false
		}

		return ctx.Value, true
	}

	if !ctx.From.AssignableTo(want) {
		return reflect.Value{}, false
	}

	v := ctx.Value
	if v.Type() != want {
		v = v.Convert(want)
	}

	return v, true
}

// ValueEncoder lets a type choose how it is written out. It is consulted
// before the hooks and before the built-in conversion.
//
// The returned value is converted to the destination as usual.
type ValueEncoder interface {
	EncodeValue() (any, error)
}

// ValueDecoder lets a type choose how it is read in. It is consulted before
// the hooks and before the built-in conversion.
//
// It is called on an addressable value, so implement it on the pointer
// receiver.
type ValueDecoder interface {
	DecodeValue(any) error
}

var (
	valueEncoderType = reflect.TypeFor[ValueEncoder]()
	valueDecoderType = reflect.TypeFor[ValueDecoder]()
)
