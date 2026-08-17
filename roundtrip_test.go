package gofret_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/rakunlabs/gofret"
)

// The round trip property is the main guarantee gofret makes and the reason
// `remain` and `inline` work in both directions: converting a value to a map
// and back must return the value unchanged.

type rtInner struct {
	Enabled bool     `gofret:"enabled"`
	Tags    []string `gofret:"tags"`
}

type rtEmbedded struct {
	Host string `gofret:"host"`
	Port int    `gofret:"port"`
}

type rtAll struct {
	Str    string             `gofret:"str"`
	Int    int                `gofret:"int"`
	Int8   int8               `gofret:"int8"`
	Uint   uint               `gofret:"uint"`
	Float  float64            `gofret:"float"`
	Bool   bool               `gofret:"bool"`
	Bytes  []byte             `gofret:"bytes"`
	Sub    rtInner            `gofret:"sub"`
	SubPtr *rtInner           `gofret:"subptr"`
	List   []rtInner          `gofret:"list"`
	Dict   map[string]int     `gofret:"dict"`
	Deep   map[string]rtInner `gofret:"deep"`
	Arr    [3]int             `gofret:"arr"`
	At     time.Time          `gofret:"at"`
	Dur    time.Duration      `gofret:"dur"`
	Base   rtEmbedded         `gofret:",inline"`
	Rest   map[string]any     `gofret:",remain"`
}

func sampleAll() rtAll {
	return rtAll{
		Str:    "hello",
		Int:    -42,
		Int8:   7,
		Uint:   99,
		Float:  1.5,
		Bool:   true,
		Bytes:  []byte("raw"),
		Sub:    rtInner{Enabled: true, Tags: []string{"a", "b"}},
		SubPtr: &rtInner{Enabled: false, Tags: []string{"c"}},
		List:   []rtInner{{Enabled: true}, {Tags: []string{"d"}}},
		Dict:   map[string]int{"x": 1, "y": 2},
		Deep:   map[string]rtInner{"k": {Enabled: true, Tags: []string{"e"}}},
		Arr:    [3]int{1, 2, 3},
		At:     time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC),
		Dur:    90 * time.Minute,
		Base:   rtEmbedded{Host: "h", Port: 8080},
		Rest:   map[string]any{"unknown": "kept"},
	}
}

func TestRoundTripThroughMap(t *testing.T) {
	orig := sampleAll()

	m, err := gofret.To[map[string]any](orig)
	if err != nil {
		t.Fatalf("to map: %v", err)
	}

	back, err := gofret.To[rtAll](m)
	if err != nil {
		t.Fatalf("to struct: %v", err)
	}

	if !reflect.DeepEqual(orig, back) {
		t.Fatalf("round trip changed the value:\n got %#v\nwant %#v", back, orig)
	}
}

func TestRoundTripThroughStruct(t *testing.T) {
	orig := sampleAll()

	back, err := gofret.To[rtAll](orig)
	if err != nil {
		t.Fatalf("struct to struct: %v", err)
	}

	if !reflect.DeepEqual(orig, back) {
		t.Fatalf("struct to struct changed the value:\n got %#v\nwant %#v", back, orig)
	}
}

func TestRoundTripIsStableAcrossCodecs(t *testing.T) {
	orig := sampleAll()

	codecs := map[string]*gofret.Codec{
		"default":      gofret.New(),
		"weak":         gofret.New(gofret.WithWeakTypes()),
		"loose keys":   gofret.New(gofret.WithLooseKeys()),
		"snake keys":   gofret.New(gofret.WithKeyFunc(gofret.SnakeCase)),
		"zero fields":  gofret.New(gofret.WithZeroFields()),
		"error unused": gofret.New(gofret.WithErrorUnused()),
	}

	for name, c := range codecs {
		t.Run(name, func(t *testing.T) {
			m, err := c.To[map[string]any](orig)
			if err != nil {
				t.Fatalf("to map: %v", err)
			}

			back, err := c.To[rtAll](m)
			if err != nil {
				t.Fatalf("to struct: %v", err)
			}

			if !reflect.DeepEqual(orig, back) {
				t.Fatalf("round trip changed the value:\n got %#v\nwant %#v", back, orig)
			}
		})
	}
}

func TestRoundTripZeroValue(t *testing.T) {
	var orig rtAll

	m, err := gofret.To[map[string]any](orig)
	if err != nil {
		t.Fatalf("to map: %v", err)
	}

	back, err := gofret.To[rtAll](m)
	if err != nil {
		t.Fatalf("to struct: %v", err)
	}

	if !reflect.DeepEqual(orig, back) {
		t.Fatalf("round trip changed the zero value:\n got %#v\nwant %#v", back, orig)
	}
}

// TestRoundTripPreservesConcreteContainerTypes guards the rule that only
// values holding something to flatten are rebuilt: a []string must not turn
// into a []any along the way.
func TestRoundTripPreservesConcreteContainerTypes(t *testing.T) {
	type payload struct {
		Strings []string       `gofret:"strings"`
		Ints    []int          `gofret:"ints"`
		Dict    map[string]int `gofret:"dict"`
	}

	orig := payload{
		Strings: []string{"a"},
		Ints:    []int{1},
		Dict:    map[string]int{"k": 1},
	}

	m, err := gofret.To[map[string]any](orig)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := m["strings"].([]string); !ok {
		t.Errorf("strings = %T, want []string", m["strings"])
	}

	if _, ok := m["ints"].([]int); !ok {
		t.Errorf("ints = %T, want []int", m["ints"])
	}

	if _, ok := m["dict"].(map[string]any); !ok {
		t.Errorf("dict = %T, want map[string]any", m["dict"])
	}
}

func TestSelfReferentialTypeTerminates(t *testing.T) {
	type node struct {
		Name string `gofret:"name"`
		Next *node  `gofret:"next"`
	}

	orig := node{Name: "a", Next: &node{Name: "b"}}

	m, err := gofret.To[map[string]any](orig)
	if err != nil {
		t.Fatal(err)
	}

	back, err := gofret.To[node](m)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(orig, back) {
		t.Fatalf("got %#v, want %#v", back, orig)
	}
}

// TestRecursiveSliceTypeTerminates covers `type list []list`, which is legal
// Go and would loop a naive type walk forever.
func TestRecursiveSliceTypeTerminates(t *testing.T) {
	type list []any

	type holder struct {
		L list `gofret:"l"`
	}

	if _, err := gofret.To[map[string]any](holder{L: list{1, "x"}}); err != nil {
		t.Fatal(err)
	}
}
