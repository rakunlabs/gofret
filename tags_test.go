package gofret_test

import (
	"reflect"
	"testing"

	"github.com/rakunlabs/gofret"
)

// ---------------------------------------------------------------------------
// `-`
// ---------------------------------------------------------------------------

type skipped struct {
	Kept    string `cfg:"kept"`
	Ignored string `cfg:"-"`
}

// TestSkipIsSymmetric pins down the fix for the biggest inconsistency in the
// predecessor, where `-` was honoured writing to a map but silently looked up
// a key literally named "-" when reading one.
func TestSkipIsSymmetric(t *testing.T) {
	t.Run("to map", func(t *testing.T) {
		got, err := gofret.To[map[string]any](skipped{Kept: "a", Ignored: "b"})
		if err != nil {
			t.Fatal(err)
		}

		want := map[string]any{"kept": "a"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("to struct", func(t *testing.T) {
		got, err := gofret.To[skipped](map[string]any{"kept": "a", "Ignored": "b", "-": "c"})
		if err != nil {
			t.Fatal(err)
		}

		if got.Ignored != "" {
			t.Fatalf("a skipped field must stay empty, got %q", got.Ignored)
		}

		if got.Kept != "a" {
			t.Fatalf("Kept = %q", got.Kept)
		}
	})
}

// ---------------------------------------------------------------------------
// omitempty
// ---------------------------------------------------------------------------

func TestOmitEmpty(t *testing.T) {
	type payload struct {
		Name  string   `cfg:"name,omitempty"`
		Count int      `cfg:"count,omitempty"`
		List  []string `cfg:"list,omitempty"`
		Ptr   *int     `cfg:"ptr,omitempty"`
		Kept  string   `cfg:"kept"`
	}

	got, err := gofret.To[map[string]any](payload{})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]any{"kept": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// ---------------------------------------------------------------------------
// inline
// ---------------------------------------------------------------------------

type inlineBase struct {
	Host string `cfg:"host"`
	Port int    `cfg:"port"`
}

type inlineOuter struct {
	Name string     `cfg:"name"`
	Base inlineBase `cfg:",inline"`
}

func TestInline(t *testing.T) {
	orig := inlineOuter{Name: "n", Base: inlineBase{Host: "h", Port: 1}}

	m, err := gofret.To[map[string]any](orig)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]any{"name": "n", "host": "h", "port": 1}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("to map: got %#v, want %#v", m, want)
	}

	back, err := gofret.To[inlineOuter](m)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(back, orig) {
		t.Fatalf("round trip: got %#v, want %#v", back, orig)
	}
}

func TestInlineThroughPointer(t *testing.T) {
	type outer struct {
		Name string      `cfg:"name"`
		Base *inlineBase `cfg:",inline"`
	}

	// Reading through a nil embedded pointer must not panic; the fields are
	// simply unreachable.
	m, err := gofret.To[map[string]any](outer{Name: "n"})
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(m, map[string]any{"name": "n"}) {
		t.Fatalf("nil inline pointer: got %#v", m)
	}

	// Writing through one allocates it.
	got, err := gofret.To[outer](map[string]any{"name": "n", "host": "h", "port": 2})
	if err != nil {
		t.Fatal(err)
	}

	if got.Base == nil || got.Base.Host != "h" || got.Base.Port != 2 {
		t.Fatalf("got %#v", got.Base)
	}
}

func TestInlineEmbeddedOption(t *testing.T) {
	type Outer struct {
		inlineBase

		Name string `cfg:"name"`
	}

	c := gofret.New(gofret.WithInlineEmbedded())

	m, err := c.To[map[string]any](Outer{inlineBase: inlineBase{Host: "h", Port: 3}, Name: "n"})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]any{"name": "n", "host": "h", "port": 3}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("got %#v, want %#v", m, want)
	}
}

// InlineBase is embedded under an exported name so it stays addressable as a
// whole field.
type InlineBase struct {
	Host string `cfg:"host"`
}

func TestEmbeddedIsNotInlinedByDefault(t *testing.T) {
	type Outer struct {
		InlineBase

		Name string `cfg:"name"`
	}

	// Without the option an embedded struct is an ordinary field named after
	// its type, which is what mapstructure and the predecessor both do.
	m, err := gofret.To[map[string]any](Outer{Name: "n"})
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := m["InlineBase"]; !ok {
		t.Fatalf("expected an InlineBase key, got %#v", m)
	}
}

// TestEmbeddedUnexportedTypeIsInlined covers the one case where embedding is
// promoted anyway: the field itself cannot be named or set, so only its
// exported fields are reachable.
func TestEmbeddedUnexportedTypeIsInlined(t *testing.T) {
	type Outer struct {
		inlineBase

		Name string `cfg:"name"`
	}

	m, err := gofret.To[map[string]any](Outer{
		inlineBase: inlineBase{Host: "h", Port: 1},
		Name:       "n",
	})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]any{"name": "n", "host": "h", "port": 1}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("got %#v, want %#v", m, want)
	}
}

func TestInlineShallowerFieldWins(t *testing.T) {
	type inner struct {
		Name string `cfg:"name"`
	}

	type outer struct {
		Name  string `cfg:"name"`
		Inner inner  `cfg:",inline"`
	}

	got, err := gofret.To[outer](map[string]any{"name": "top"})
	if err != nil {
		t.Fatal(err)
	}

	if got.Name != "top" {
		t.Fatalf("the shallower field must win, got outer.Name=%q inner.Name=%q", got.Name, got.Inner.Name)
	}

	if got.Inner.Name != "" {
		t.Fatalf("the shadowed field must stay empty, got %q", got.Inner.Name)
	}
}

func TestInlineCycleTerminates(t *testing.T) {
	type node struct {
		Name string `cfg:"name"`
		Next *node  `cfg:",inline"`
	}

	// Analysis must not loop forever on a type that inlines itself.
	got, err := gofret.To[node](map[string]any{"name": "n"})
	if err != nil {
		t.Fatal(err)
	}

	if got.Name != "n" {
		t.Fatalf("got %#v", got)
	}
}

func TestInlineRequiresStruct(t *testing.T) {
	type bad struct {
		Oops string `cfg:",inline"`
	}

	if _, err := gofret.To[map[string]any](bad{}); err == nil {
		t.Fatal("inlining a non-struct must be reported")
	}
}

// ---------------------------------------------------------------------------
// remain
// ---------------------------------------------------------------------------

type withRemain struct {
	Name string         `cfg:"name"`
	Rest map[string]any `cfg:",remain"`
}

func TestRemainCapturesUnknownKeys(t *testing.T) {
	got, err := gofret.To[withRemain](map[string]any{
		"name":  "n",
		"extra": 1,
		"other": "x",
	})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]any{"extra": 1, "other": "x"}
	if !reflect.DeepEqual(got.Rest, want) {
		t.Fatalf("got %#v, want %#v", got.Rest, want)
	}
}

// TestRemainRoundTrips is the reason `remain` works in both directions:
// without writing the captured keys back out, a round trip would lose them.
func TestRemainRoundTrips(t *testing.T) {
	orig := map[string]any{"name": "n", "extra": 1, "other": "x"}

	mid, err := gofret.To[withRemain](orig)
	if err != nil {
		t.Fatal(err)
	}

	back, err := gofret.To[map[string]any](mid)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(back, orig) {
		t.Fatalf("round trip lost data:\n got %#v\nwant %#v", back, orig)
	}
}

func TestRemainMustBeUnique(t *testing.T) {
	type bad struct {
		A map[string]any `cfg:",remain"`
		B map[string]any `cfg:",remain"`
	}

	if _, err := gofret.To[bad](map[string]any{}); err == nil {
		t.Fatal("two remain fields must be reported")
	}
}

func TestRemainMustBeMap(t *testing.T) {
	type bad struct {
		Rest string `cfg:",remain"`
	}

	if _, err := gofret.To[bad](map[string]any{"x": 1}); err == nil {
		t.Fatal("a non-map remain field must be reported")
	}
}

// ---------------------------------------------------------------------------
// string
// ---------------------------------------------------------------------------

func TestStringOption(t *testing.T) {
	type payload struct {
		Count int     `cfg:"count,string"`
		Ratio float64 `cfg:"ratio,string"`
		Flag  bool    `cfg:"flag,string"`
	}

	orig := payload{Count: 42, Ratio: 1.5, Flag: true}

	m, err := gofret.To[map[string]any](orig)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]any{"count": "42", "ratio": "1.5", "flag": "true"}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("to map: got %#v, want %#v", m, want)
	}

	// The strict codec still reads it back, because `string` says this field
	// travels as text.
	back, err := gofret.To[payload](m)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(back, orig) {
		t.Fatalf("round trip: got %#v, want %#v", back, orig)
	}
}

func TestStringOptionUsesStringer(t *testing.T) {
	type payload struct {
		Level level `cfg:"level,string"`
	}

	m, err := gofret.To[map[string]any](payload{Level: 2})
	if err != nil {
		t.Fatal(err)
	}

	if m["level"] != "high" {
		t.Fatalf("got %#v", m)
	}
}

type level int

func (l level) String() string {
	if l >= 2 {
		return "high"
	}

	return "low"
}

// ---------------------------------------------------------------------------
// deref
// ---------------------------------------------------------------------------

func TestDeref(t *testing.T) {
	type payload struct {
		Set   *int `cfg:"set,deref"`
		Unset *int `cfg:"unset,deref"`
		Plain *int `cfg:"plain"`
	}

	n := 5

	got, err := gofret.To[map[string]any](payload{Set: &n})
	if err != nil {
		t.Fatal(err)
	}

	if got["set"] != 5 {
		t.Fatalf("set = %#v, want 5", got["set"])
	}

	// A nil pointer becomes the zero value rather than disappearing.
	if got["unset"] != 0 {
		t.Fatalf("unset = %#v, want 0", got["unset"])
	}

	// Without `deref` the pointer is carried across as it stands, typed nil
	// and all, so the shape of the output mirrors the input.
	p, ok := got["plain"].(*int)
	if !ok || p != nil {
		t.Fatalf("plain = %#v, want a nil *int", got["plain"])
	}
}

func TestDerefPointersOption(t *testing.T) {
	type payload struct {
		N *int `cfg:"n"`
	}

	n := 5

	c := gofret.New(gofret.WithDerefPointers())

	got, err := c.To[map[string]any](payload{N: &n})
	if err != nil {
		t.Fatal(err)
	}

	if got["n"] != 5 {
		t.Fatalf("got %#v", got)
	}
}

// ---------------------------------------------------------------------------
// keep
// ---------------------------------------------------------------------------

func TestKeep(t *testing.T) {
	type inner struct {
		A int `cfg:"a"`
	}

	type payload struct {
		Converted inner `cfg:"converted"`
		Untouched inner `cfg:"untouched,keep"`
	}

	got, err := gofret.To[map[string]any](payload{
		Converted: inner{A: 1},
		Untouched: inner{A: 2},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := got["converted"].(map[string]any); !ok {
		t.Fatalf("converted = %#v, want a map", got["converted"])
	}

	if _, ok := got["untouched"].(inner); !ok {
		t.Fatalf("untouched = %#v, want the original struct", got["untouched"])
	}
}

func TestShallowOption(t *testing.T) {
	type inner struct {
		A int `cfg:"a"`
	}

	type payload struct {
		Sub inner `cfg:"sub"`
	}

	c := gofret.New(gofret.WithShallow())

	got, err := c.To[map[string]any](payload{Sub: inner{A: 1}})
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := got["sub"].(inner); !ok {
		t.Fatalf("sub = %#v, want the original struct", got["sub"])
	}
}
