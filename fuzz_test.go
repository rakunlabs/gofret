package gofret_test

import (
	"testing"

	"github.com/rakunlabs/gofret"
)

type fuzzTarget struct {
	Str   string         `cfg:"str"`
	Int   int            `cfg:"int"`
	Int8  int8           `cfg:"int8"`
	Uint  uint16         `cfg:"uint"`
	Float float64        `cfg:"float"`
	Bool  bool           `cfg:"bool"`
	List  []string       `cfg:"list"`
	Dict  map[string]int `cfg:"dict"`
	Sub   *fuzzSub       `cfg:"sub"`
	Rest  map[string]any `cfg:",remain"`
}

type fuzzSub struct {
	Nested string `cfg:"nested"`
}

// FuzzToStruct feeds arbitrary maps at a struct. Nothing it can produce may
// panic; every rejection has to arrive as an error.
func FuzzToStruct(f *testing.F) {
	f.Add("str", "value", 0)
	f.Add("int", "42", 1)
	f.Add("int8", "999999", 2)
	f.Add("list", "a", 3)
	f.Add("dict", "1", 4)
	f.Add("sub", "x", 5)
	f.Add("", "", 6)

	codecs := []*gofret.Codec{
		gofret.New(),
		gofret.New(gofret.WithStrictTypes(), gofret.WithStrictKeys()),
		gofret.New(gofret.WithWeakTypes(), gofret.WithLooseKeys(), gofret.WithErrorUnused()),
		gofret.New(gofret.WithZeroFields(), gofret.WithFailFast()),
	}

	f.Fuzz(func(t *testing.T, key, value string, shape int) {
		var v any = value

		switch shape % 7 {
		case 1:
			v = []any{value}
		case 2:
			v = map[string]any{key: value}
		case 3:
			v = map[string]any{"nested": value}
		case 4:
			v = nil
		case 5:
			v = []any{map[string]any{key: value}}
		case 6:
			v = map[string]any{key: map[string]any{key: value}}
		}

		in := map[string]any{key: v}

		for _, c := range codecs {
			var out fuzzTarget

			_ = c.ToInto(in, &out)

			// Whatever came out must survive a trip back to a map.
			_, _ = c.To[map[string]any](out)
		}
	})
}

// FuzzRoundTrip checks that a value built from fuzzed input still satisfies
// the round trip property.
func FuzzRoundTrip(f *testing.F) {
	f.Add("a", 1, true, 1.5)
	f.Add("", 0, false, 0.0)
	f.Add("x", -1, true, -2.5)

	c := gofret.New()

	f.Fuzz(func(t *testing.T, str string, n int, b bool, fl float64) {
		orig := fuzzTarget{
			Str:   str,
			Int:   n,
			Bool:  b,
			Float: fl,
			List:  []string{str},
			Dict:  map[string]int{str: n},
			Sub:   &fuzzSub{Nested: str},
		}

		m, err := c.To[map[string]any](orig)
		if err != nil {
			t.Fatalf("to map: %v", err)
		}

		back, err := c.To[fuzzTarget](m)
		if err != nil {
			t.Fatalf("to struct: %v", err)
		}

		if back.Str != orig.Str || back.Int != orig.Int || back.Bool != orig.Bool {
			t.Fatalf("round trip changed the value:\n got %#v\nwant %#v", back, orig)
		}

		// NaN is never equal to itself, so compare the bits instead.
		if (back.Float != orig.Float) && !(isNaN(back.Float) && isNaN(orig.Float)) {
			t.Fatalf("float changed: got %v, want %v", back.Float, orig.Float)
		}

		if back.Sub == nil || back.Sub.Nested != orig.Sub.Nested {
			t.Fatalf("nested value changed: got %#v", back.Sub)
		}
	})
}

func isNaN(f float64) bool { return f != f }
