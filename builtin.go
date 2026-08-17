package gofret

import (
	"fmt"
	"reflect"
	"time"
)

// DurationHook parses a string into a time.Duration, so "1h30m" reaches a
// time.Duration field.
//
// Numbers are left to the ordinary integer conversion, where they are taken
// as nanoseconds, matching what time.Duration itself means.
//
// The reverse needs no hook: time.Duration is a fmt.Stringer, so writing one
// into a string destination already yields "1h30m0s".
var DurationHook Hook = HookBetween(func(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrUnconvertible, err)
	}

	return d, nil
})

// TimeHook parses a string into a time.Time, trying each layout in turn.
//
// With no layouts it accepts RFC 3339. A time.Time field already understands
// RFC 3339 on its own through encoding.TextUnmarshaler, so reach for this
// only when the input uses some other layout.
func TimeHook(layouts ...string) Hook {
	if len(layouts) == 0 {
		layouts = []string{time.RFC3339}
	}

	return HookBetween(func(s string) (time.Time, error) {
		if s == "" {
			return time.Time{}, nil
		}

		var err error

		for _, layout := range layouts {
			var t time.Time

			if t, err = time.Parse(layout, s); err == nil {
				return t, nil
			}
		}

		return time.Time{}, fmt.Errorf("%w: %q matches none of the layouts %q", ErrUnconvertible, s, layouts)
	})
}

// TimeFormatHook renders a time.Time with the given layout on the way out.
// With no layout it uses RFC 3339.
func TimeFormatHook(layout string) Hook {
	if layout == "" {
		layout = time.RFC3339
	}

	return HookFrom(func(t time.Time) (any, error) {
		return t.Format(layout), nil
	})
}

// NilHook replaces a nil pointer, map or slice with the zero value of the
// type it points at, so a destination never sees an untyped nil.
var NilHook Hook = func(ctx HookCtx) (any, error) {
	if ctx.From == nil || !isNilLike(ctx.Value) {
		return nil, ErrSkip
	}

	if ctx.From.Kind() != reflect.Pointer {
		return nil, ErrSkip
	}

	return reflect.Zero(ctx.From.Elem()).Interface(), nil
}
