package gofret_test

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/rakunlabs/gofret"
)

type nested struct {
	Ports []int `gofret:"ports"`
}

type errPayload struct {
	Name string `gofret:"name"`
	Sub  nested `gofret:"sub"`
}

func TestErrorCarriesPath(t *testing.T) {
	in := map[string]any{
		"sub": map[string]any{"ports": []any{1, "nope", 3}},
	}

	err := gofret.ToInto(in, &errPayload{})
	if err == nil {
		t.Fatal("expected an error")
	}

	var ce *gofret.Error
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want a *gofret.Error", err)
	}

	if ce.Path != "sub.ports[1]" {
		t.Fatalf("Path = %q, want %q", ce.Path, "sub.ports[1]")
	}

	if ce.To == nil || ce.To.Kind().String() != "int" {
		t.Fatalf("To = %v, want int", ce.To)
	}

	if !errors.Is(err, gofret.ErrUnconvertible) {
		t.Fatalf("err = %v, want it to wrap ErrUnconvertible", err)
	}
}

func TestAllErrorsAreCollected(t *testing.T) {
	type payload struct {
		A int `gofret:"a"`
		B int `gofret:"b"`
		C int `gofret:"c"`
	}

	in := map[string]any{"a": "x", "b": "y", "c": "z"}

	err := gofret.ToInto(in, &payload{})
	if err == nil {
		t.Fatal("expected an error")
	}

	for _, key := range []string{"a", "b", "c"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error does not mention %q:\n%v", key, err)
		}
	}
}

func TestWithFailFast(t *testing.T) {
	type payload struct {
		A int `gofret:"a"`
		B int `gofret:"b"`
		C int `gofret:"c"`
	}

	in := map[string]any{"a": "x", "b": "y", "c": "z"}

	c := gofret.New(gofret.WithFailFast())

	err := c.ToInto(in, &payload{})
	if err == nil {
		t.Fatal("expected an error")
	}

	if n := countJoined(err); n != 1 {
		t.Fatalf("collected %d errors, want 1", n)
	}
}

func TestWithMaxErrors(t *testing.T) {
	type payload struct {
		A int `gofret:"a"`
		B int `gofret:"b"`
		C int `gofret:"c"`
	}

	in := map[string]any{"a": "x", "b": "y", "c": "z"}

	c := gofret.New(gofret.WithMaxErrors(2))

	err := c.ToInto(in, &payload{})
	if err == nil {
		t.Fatal("expected an error")
	}

	if n := countJoined(err); n > 2 {
		t.Fatalf("collected %d errors, want at most 2", n)
	}
}

func countJoined(err error) int {
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		return len(joined.Unwrap())
	}

	return 1
}

func TestWithErrorUnused(t *testing.T) {
	type payload struct {
		Name string `gofret:"name"`
	}

	in := map[string]any{"name": "n", "typo": 1}

	if _, err := gofret.To[payload](in); err != nil {
		t.Fatalf("unused keys are ignored by default: %v", err)
	}

	c := gofret.New(gofret.WithErrorUnused())

	_, err := c.To[payload](in)
	if err == nil {
		t.Fatal("expected an error about the unused key")
	}

	if !errors.Is(err, gofret.ErrUnusedKeys) {
		t.Fatalf("err = %v, want it to wrap ErrUnusedKeys", err)
	}

	if !strings.Contains(err.Error(), "typo") {
		t.Fatalf("the offending key should be named:\n%v", err)
	}
}

func TestErrorUnusedIsSatisfiedByRemain(t *testing.T) {
	c := gofret.New(gofret.WithErrorUnused())

	if _, err := c.To[withRemain](map[string]any{"name": "n", "extra": 1}); err != nil {
		t.Fatalf("a remain field absorbs the key: %v", err)
	}
}

func TestToIntoRejectsNonPointer(t *testing.T) {
	var out struct{}

	for _, bad := range []any{out, nil, (*int)(nil)} {
		if err := gofret.ToInto(map[string]any{}, bad); !errors.Is(err, gofret.ErrNotPointer) {
			t.Errorf("ToInto(%#v) = %v, want ErrNotPointer", bad, err)
		}
	}
}

func TestNoPanicOnOddInput(t *testing.T) {
	// The predecessor panicked on any of these. A library should not.
	type payload struct {
		Name string `gofret:"name"`
	}

	inputs := []any{nil, 42, "text", []int{1, 2}, make(chan int)}

	for _, in := range inputs {
		var out payload

		_ = gofret.ToInto(in, &out)
	}

	outputs := []any{new(int), new(string), new([]int), new(map[string]any)}

	for _, out := range outputs {
		_ = gofret.ToInto(payload{Name: "n"}, out)
	}
}

func TestOverflowIsReported(t *testing.T) {
	type payload struct {
		Small int8 `gofret:"small"`
	}

	_, err := gofret.To[payload](map[string]any{"small": 5000})
	if !errors.Is(err, gofret.ErrOverflow) {
		t.Fatalf("err = %v, want ErrOverflow", err)
	}
}

func TestFractionalFloatToIntIsReported(t *testing.T) {
	type payload struct {
		N int `gofret:"n"`
	}

	// A whole number arriving as a float, which is how JSON delivers it, is
	// fine.
	got, err := gofret.To[payload](map[string]any{"n": float64(5)})
	if err != nil {
		t.Fatalf("5.0 should convert cleanly: %v", err)
	}

	if got.N != 5 {
		t.Fatalf("got %#v", got)
	}

	// Truncating one silently would lose information, so it needs weak types.
	if _, err := gofret.To[payload](map[string]any{"n": 5.5}); err == nil {
		t.Fatal("5.5 must not silently truncate to 5")
	}

	c := gofret.New(gofret.WithWeakTypes())

	got, err = c.To[payload](map[string]any{"n": 5.5})
	if err != nil {
		t.Fatal(err)
	}

	if got.N != 5 {
		t.Fatalf("got %#v", got)
	}
}

func TestErrorMessageShape(t *testing.T) {
	e := &gofret.Error{
		Path: "a.b[0]",
		Err:  gofret.ErrUnconvertible,
	}

	got := e.Error()
	if !strings.HasPrefix(got, "gofret: a.b[0]: ") {
		t.Fatalf("Error() = %q", got)
	}

	if !errors.Is(e, gofret.ErrUnconvertible) {
		t.Fatal("Error must unwrap to its cause")
	}
}

// TestErrorWorksWithAsType checks that *Error plays properly with the
// standard errors.AsType, so gofret needs no helper of its own.
func TestErrorWorksWithAsType(t *testing.T) {
	in := map[string]any{
		"sub": map[string]any{"ports": []any{1, "nope", 3}},
	}

	err := gofret.ToInto(in, &errPayload{})

	ce, ok := errors.AsType[*gofret.Error](err)
	if !ok {
		t.Fatalf("errors.AsType found nothing in %v", err)
	}

	if ce.Path != "sub.ports[1]" {
		t.Fatalf("Path = %q", ce.Path)
	}

	if _, ok := errors.AsType[*strconv.NumError](err); ok {
		t.Fatal("there is no NumError in this tree")
	}
}

func TestErrorsListsEveryFailure(t *testing.T) {
	type payload struct {
		A int `gofret:"a"`
		B int `gofret:"b"`
		C int `gofret:"c"`
	}

	in := map[string]any{"a": "x", "b": "y", "c": "z"}

	err := gofret.ToInto(in, &payload{})

	all := gofret.Errors(err)
	if len(all) != 3 {
		t.Fatalf("Errors() found %d failures, want 3: %v", len(all), err)
	}

	seen := map[string]bool{}
	for _, ce := range all {
		seen[ce.Path] = true
	}

	for _, want := range []string{"a", "b", "c"} {
		if !seen[want] {
			t.Errorf("no failure reported for %q, got %v", want, seen)
		}
	}
}

func TestErrorsOnSingleAndNil(t *testing.T) {
	if got := gofret.Errors(nil); got != nil {
		t.Fatalf("Errors(nil) = %v", got)
	}

	if got := gofret.Errors(errors.New("plain")); got != nil {
		t.Fatalf("Errors(plain) = %v", got)
	}

	type payload struct {
		A int `gofret:"a"`
	}

	err := gofret.ToInto(map[string]any{"a": "x"}, &payload{})

	if got := gofret.Errors(err); len(got) != 1 {
		t.Fatalf("Errors() = %v, want one entry", got)
	}
}

// TestErrorOrderIsDeterministic pins down a guarantee that is easy to lose:
// conversion walks the destination fields in declaration order rather than
// following Go's randomised map iteration, so the same input always reports
// the same failures in the same order.
func TestErrorOrderIsDeterministic(t *testing.T) {
	type payload struct {
		A int `gofret:"a"`
		B int `gofret:"b"`
		C int `gofret:"c"`
		D int `gofret:"d"`
		E int `gofret:"e"`
	}

	in := map[string]any{"a": "x", "b": "x", "c": "x", "d": "x", "e": "x"}

	want := []string{"a", "b", "c", "d", "e"}

	for range 50 {
		err := gofret.ToInto(in, &payload{})

		var got []string
		for _, ce := range gofret.Errors(err) {
			got = append(got, ce.Path)
		}

		if !slices.Equal(got, want) {
			t.Fatalf("error order = %q, want %q", got, want)
		}
	}
}

func TestMetadataOrderIsDeterministic(t *testing.T) {
	type payload struct {
		A string `gofret:"a"`
		B string `gofret:"b"`
		C string `gofret:"c"`
		D string `gofret:"d"`
	}

	in := map[string]any{"a": "1", "b": "2", "c": "3", "d": "4"}

	want := []string{"a", "b", "c", "d"}

	for range 50 {
		var (
			md  gofret.Metadata
			out payload
		)

		if err := gofret.New().ToIntoMeta(in, &out, &md); err != nil {
			t.Fatal(err)
		}

		if !slices.Equal(md.Keys, want) {
			t.Fatalf("Keys = %q, want %q", md.Keys, want)
		}
	}
}

// TestExactKeyBeatsNormalized settles the other source of map-order
// dependence: when two keys fold to the same field, the exact name wins.
func TestExactKeyBeatsNormalized(t *testing.T) {
	type payload struct {
		Host string `gofret:"host"`
	}

	in := map[string]any{"host": "exact", "HOST": "folded"}

	for range 50 {
		got, err := gofret.To[payload](in)
		if err != nil {
			t.Fatal(err)
		}

		if got.Host != "exact" {
			t.Fatalf("Host = %q, want the exact key to win", got.Host)
		}
	}
}
