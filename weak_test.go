package gofret_test

import (
	"reflect"
	"testing"

	"github.com/rakunlabs/gofret"
)

// TestWeakMatrix walks every source kind against every scalar destination, in
// both modes, so no branch of the conversion table goes unexercised.
func TestWeakMatrix(t *testing.T) {
	sources := map[string]any{
		"bool":    true,
		"int":     int(1),
		"int8":    int8(1),
		"int64":   int64(1),
		"uint":    uint(1),
		"uint8":   uint8(1),
		"uint64":  uint64(1),
		"float32": float32(1),
		"float64": float64(1),
		"string":  "1",
		"bytes":   []byte("1"),
	}

	dests := map[string]func() any{
		"bool":       func() any { return new(bool) },
		"int":        func() any { return new(int) },
		"int8":       func() any { return new(int8) },
		"int64":      func() any { return new(int64) },
		"uint":       func() any { return new(uint) },
		"uint8":      func() any { return new(uint8) },
		"uint64":     func() any { return new(uint64) },
		"float32":    func() any { return new(float32) },
		"float64":    func() any { return new(float64) },
		"string":     func() any { return new(string) },
		"complex128": func() any { return new(complex128) },
	}

	strict := gofret.New()
	weak := gofret.New(gofret.WithWeakTypes())

	for sn, sv := range sources {
		for dn, mk := range dests {
			t.Run(sn+"_to_"+dn, func(t *testing.T) {
				// Neither mode may panic, whatever the pairing.
				_ = strict.ToInto(sv, mk())

				out := mk()

				// Weak mode must succeed for every scalar pairing here; the
				// value 1 is representable in all of them.
				err := weak.ToInto(sv, out)

				switch {
				case sn == "bytes" && dn != "string":
					// A byte slice only has a weak reading as text.
					if err == nil {
						t.Fatalf("[]byte to %s should not convert", dn)
					}
				case dn == "complex128" && sn == "string":
					if err == nil {
						t.Fatal("a string should not become a complex number")
					}
				case dn == "complex128" && sn == "bool":
					if err == nil {
						t.Fatal("a bool should not become a complex number")
					}
				default:
					if err != nil {
						t.Fatalf("%s to %s: %v", sn, dn, err)
					}
				}
			})
		}
	}
}

func TestStrictRejectsCrossKind(t *testing.T) {
	type pair struct {
		in  any
		out func() any
	}

	pairs := []pair{
		{"1", func() any { return new(int) }},
		{"1", func() any { return new(float64) }},
		{"true", func() any { return new(bool) }},
		{1, func() any { return new(string) }},
		{1, func() any { return new(bool) }},
		{true, func() any { return new(int) }},
		{true, func() any { return new(string) }},
	}

	c := gofret.New()

	for _, p := range pairs {
		out := p.out()

		if err := c.ToInto(p.in, out); err == nil {
			t.Errorf("%T(%v) to %T should be refused in strict mode", p.in, p.in, out)
		}
	}
}

func TestStrictAcceptsLosslessNumeric(t *testing.T) {
	type pair struct {
		in   any
		out  func() any
		want any
	}

	pairs := []pair{
		{int(1), func() any { return new(int64) }, int64(1)},
		{int8(1), func() any { return new(int) }, 1},
		{uint8(1), func() any { return new(int64) }, int64(1)},
		{int(1), func() any { return new(uint) }, uint(1)},
		{int(1), func() any { return new(float64) }, float64(1)},
		{float64(1), func() any { return new(int) }, 1},
		{float32(1), func() any { return new(float64) }, float64(1)},
	}

	c := gofret.New()

	for _, p := range pairs {
		out := p.out()

		if err := c.ToInto(p.in, out); err != nil {
			t.Errorf("%T(%v): %v", p.in, p.in, err)

			continue
		}

		got := reflect.ValueOf(out).Elem().Interface()
		if !reflect.DeepEqual(got, p.want) {
			t.Errorf("%T(%v) = %#v, want %#v", p.in, p.in, got, p.want)
		}
	}
}

func TestNamedScalarTypes(t *testing.T) {
	type myInt int

	type myString string

	type payload struct {
		N myInt    `gofret:"n"`
		S myString `gofret:"s"`
	}

	got, err := gofret.To[payload](map[string]any{"n": 5, "s": "x"})
	if err != nil {
		t.Fatal(err)
	}

	if got.N != 5 || got.S != "x" {
		t.Fatalf("got %#v", got)
	}

	m, err := gofret.To[map[string]any](got)
	if err != nil {
		t.Fatal(err)
	}

	// Named types keep their identity on the way out.
	if _, ok := m["n"].(myInt); !ok {
		t.Fatalf("n = %T, want myInt", m["n"])
	}
}
