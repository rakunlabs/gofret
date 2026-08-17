package gofret

import (
	"reflect"
	"sync"
)

// Codec converts values between shapes according to a fixed configuration.
//
// A Codec is immutable once built and safe for concurrent use. It caches the
// analysis of every struct type it sees, so reuse one instead of calling New
// on every conversion.
type Codec struct {
	cfg config

	// cache maps reflect.Type to *structInfo. It is keyed per codec because
	// the analysis depends on the tag name, key function and normalizer.
	cache sync.Map

	// generics maps reflect.Type to bool, memoising needsGeneric.
	generics sync.Map
}

// New builds a Codec from the given options.
func New(opts ...Option) *Codec {
	cfg := defaultConfig()

	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	return &Codec{cfg: cfg}
}

// std is the zero-configuration codec backing the package-level functions.
var std = New()

// To converts in to a value of type T.
//
// T decides what happens: converting to a struct reads a map, converting to
// map[string]any writes one, and converting between two structs copies field
// by field.
//
//	cfg, err := c.To[Config](data)              // map    -> struct
//	m, err := c.To[map[string]any](cfg)         // struct -> map
//
// On error the returned value holds whatever was converted before the failure;
// treat it as unusable unless err is nil.
func (c *Codec) To[T any](in any) (T, error) {
	var out T

	err := c.ToInto(in, &out)

	return out, err
}

// ToInto converts in and writes the result through out, which must be a
// non-nil pointer.
func (c *Codec) ToInto(in, out any) error {
	return c.run(in, out, nil)
}

// ToIntoMeta is ToInto and additionally records what happened into md, which
// may be nil.
func (c *Codec) ToIntoMeta(in, out any, md *Metadata) error {
	return c.run(in, out, md)
}

func (c *Codec) run(in, out any, md *Metadata) error {
	rv := reflect.ValueOf(out)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return ErrNotPointer
	}

	st := &state{
		c:    c,
		meta: md,
		weak: c.cfg.weakTypes,
		errs: errorList{max: c.cfg.maxErrors, failFast: c.cfg.failFast},
	}

	st.convert(reflect.ValueOf(in), rv.Elem(), Tag{})

	return st.errs.err()
}

// To converts in to a value of type T using the default configuration.
//
// Build a Codec with New when you need options or want the type cache to be
// reused across calls.
func To[T any](in any) (T, error) { return std.To[T](in) }

// ToInto converts in and writes the result through out, which must be a
// non-nil pointer, using the default configuration.
func ToInto(in, out any) error { return std.ToInto(in, out) }
