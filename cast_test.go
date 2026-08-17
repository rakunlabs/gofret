package gofret_test

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/rakunlabs/gofret"
)

type ptrStringer struct{ n int }

func (p *ptrStringer) String() string { return fmt.Sprintf("n=%d", p.n) }

type ptrEncoder struct{ n int }

func (p *ptrEncoder) EncodeValue() (any, error) { return p.n * 2, nil }

type failingEncoder struct{}

func (failingEncoder) EncodeValue() (any, error) { return nil, errors.New("encode boom") }

type skippingEncoder struct {
	N int `cfg:"n"`
}

func (skippingEncoder) EncodeValue() (any, error) { return nil, gofret.ErrSkip }

func TestStringerOnPointerReceiver(t *testing.T) {
	type payload struct {
		P ptrStringer `cfg:"p,string"`
	}

	got, err := gofret.To[map[string]any](payload{P: ptrStringer{n: 3}})
	if err != nil {
		t.Fatal(err)
	}

	if got["p"] != "n=3" {
		t.Fatalf("got %#v", got["p"])
	}
}

func TestValueEncoderOnPointerReceiver(t *testing.T) {
	type payload struct {
		P ptrEncoder `cfg:"p"`
	}

	got, err := gofret.To[map[string]any](payload{P: ptrEncoder{n: 3}})
	if err != nil {
		t.Fatal(err)
	}

	if got["p"] != 6 {
		t.Fatalf("got %#v", got["p"])
	}
}

func TestValueEncoderErrorPropagates(t *testing.T) {
	type payload struct {
		P failingEncoder `cfg:"p"`
	}

	_, err := gofret.To[map[string]any](payload{})
	if err == nil {
		t.Fatal("a failing EncodeValue must be reported")
	}
}

// TestValueEncoderCanDecline shows the escape hatch: returning ErrSkip means
// "handle me the usual way".
func TestValueEncoderCanDecline(t *testing.T) {
	type payload struct {
		P skippingEncoder `cfg:"p"`
	}

	got, err := gofret.To[map[string]any](payload{P: skippingEncoder{N: 4}})
	if err != nil {
		t.Fatal(err)
	}

	sub, ok := got["p"].(map[string]any)
	if !ok || sub["n"] != 4 {
		t.Fatalf("got %#v", got["p"])
	}
}

func TestErrorRendersAsString(t *testing.T) {
	type payload struct {
		Err error `cfg:"err,string"`
	}

	got, err := gofret.To[map[string]any](payload{Err: errors.New("bad")})
	if err != nil {
		t.Fatal(err)
	}

	if got["err"] != "bad" {
		t.Fatalf("got %#v", got["err"])
	}
}

func TestNilRendersAsEmptyString(t *testing.T) {
	type payload struct {
		Err error   `cfg:"err,string"`
		Ptr *int    `cfg:"ptr,string"`
		Any any     `cfg:"any,string"`
		Fn  *string `cfg:"fn,string"`
	}

	got, err := gofret.To[map[string]any](payload{})
	if err != nil {
		t.Fatal(err)
	}

	for _, k := range []string{"err", "ptr", "any", "fn"} {
		if got[k] != "" {
			t.Errorf("%s = %#v, want an empty string", k, got[k])
		}
	}
}

func TestTextUnmarshalerOnCommonTypes(t *testing.T) {
	type payload struct {
		IP net.IP `cfg:"ip"`
	}

	got, err := gofret.To[payload](map[string]any{"ip": "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}

	if !got.IP.Equal(net.ParseIP("192.0.2.1")) {
		t.Fatalf("got %v", got.IP)
	}
}

func TestTextMarshalerError(t *testing.T) {
	type payload struct {
		At badText `cfg:"at"`
	}

	if _, err := gofret.To[map[string]string](payload{}); err == nil {
		t.Fatal("a failing MarshalText must be reported")
	}
}

type badText struct{}

func (badText) MarshalText() ([]byte, error) { return nil, errors.New("marshal boom") }

func TestTextUnmarshalerError(t *testing.T) {
	type payload struct {
		At time.Time `cfg:"at"`
	}

	if _, err := gofret.To[payload](map[string]any{"at": "not a time"}); err == nil {
		t.Fatal("a failing UnmarshalText must be reported")
	}
}

func TestComplexConversions(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want complex128
	}{
		{"complex", complex(1.0, 2.0), complex(1, 2)},
		{"int", 3, complex(3, 0)},
		{"uint", uint(4), complex(4, 0)},
		{"float", 2.5, complex(2.5, 0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out complex128

			if err := gofret.ToInto(tt.in, &out); err != nil {
				t.Fatal(err)
			}

			if out != tt.want {
				t.Fatalf("got %v, want %v", out, tt.want)
			}
		})
	}

	var out complex64

	if err := gofret.ToInto("x", &out); err == nil {
		t.Fatal("a string must not become a complex number")
	}
}

func TestUintFromNegativeUnderWeakTypes(t *testing.T) {
	var out uint8

	c := gofret.New(gofret.WithWeakTypes())

	// Weak typing lets a negative wrap, matching the C-like behaviour callers
	// of the predecessor relied on.
	if err := c.ToInto(-1, &out); err != nil {
		t.Fatal(err)
	}

	if out != 255 {
		t.Fatalf("got %d, want 255", out)
	}
}

func TestFloatFromNegativeToUint(t *testing.T) {
	var out uint

	c := gofret.New(gofret.WithStrictTypes())

	if err := c.ToInto(-1.0, &out); !errors.Is(err, gofret.ErrOverflow) {
		t.Fatalf("err = %v, want ErrOverflow", err)
	}
}

func TestUnsupportedDestination(t *testing.T) {
	var out chan int

	if err := gofret.ToInto(5, &out); err == nil {
		t.Fatal("writing an int into a channel must be reported")
	}

	var fn func()

	if err := gofret.ToInto(5, &fn); err == nil {
		t.Fatal("writing an int into a func must be reported")
	}
}

func TestSameFuncTypePassesThrough(t *testing.T) {
	var out func() int

	want := func() int { return 7 }

	if err := gofret.ToInto(want, &out); err != nil {
		t.Fatal(err)
	}

	if out == nil || out() != 7 {
		t.Fatal("a func of the same type should be carried across")
	}
}

func TestRunesAndBytesToString(t *testing.T) {
	c := gofret.New(gofret.WithWeakTypes())

	var out string

	if err := c.ToInto([]rune("abc"), &out); err != nil {
		t.Fatal(err)
	}

	if out != "abc" {
		t.Fatalf("got %q", out)
	}
}

func TestNonStringMapKeyInPath(t *testing.T) {
	type payload struct {
		N int `cfg:"1"`
	}

	// A non-string key still has to produce a readable path.
	got, err := gofret.To[payload](map[int]any{1: 5})
	if err != nil {
		t.Fatal(err)
	}

	if got.N != 5 {
		t.Fatalf("got %#v", got)
	}
}

func TestTimeFormatHook(t *testing.T) {
	type payload struct {
		At time.Time `cfg:"at"`
	}

	at := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

	c := gofret.New(gofret.WithHooks(gofret.TimeFormatHook("2006-01-02")))

	got, err := c.To[map[string]any](payload{At: at})
	if err != nil {
		t.Fatal(err)
	}

	if got["at"] != "2026-08-17" {
		t.Fatalf("got %#v", got["at"])
	}

	// The empty layout falls back to RFC 3339.
	c = gofret.New(gofret.WithHooks(gofret.TimeFormatHook("")))

	got, err = c.To[map[string]any](payload{At: at})
	if err != nil {
		t.Fatal(err)
	}

	if got["at"] != "2026-08-17T12:00:00Z" {
		t.Fatalf("got %#v", got["at"])
	}
}

func TestNilHook(t *testing.T) {
	type payload struct {
		N *int `cfg:"n"`
	}

	c := gofret.New(gofret.WithHooks(gofret.NilHook))

	got, err := c.To[map[string]any](payload{})
	if err != nil {
		t.Fatal(err)
	}

	if got["n"] != 0 {
		t.Fatalf("got %#v, want the zero value in place of nil", got["n"])
	}
}

func TestEmptyDurationString(t *testing.T) {
	type payload struct {
		D time.Duration `cfg:"d"`
	}

	c := gofret.New(gofret.WithHooks(gofret.DurationHook))

	got, err := c.To[payload](map[string]any{"d": ""})
	if err != nil {
		t.Fatal(err)
	}

	if got.D != 0 {
		t.Fatalf("got %v", got.D)
	}
}

func TestEmptyTimeString(t *testing.T) {
	type payload struct {
		At time.Time `cfg:"at"`
	}

	c := gofret.New(gofret.WithHooks(gofret.TimeHook()))

	got, err := c.To[payload](map[string]any{"at": ""})
	if err != nil {
		t.Fatal(err)
	}

	if !got.At.IsZero() {
		t.Fatalf("got %v", got.At)
	}
}

func TestArrayFromString(t *testing.T) {
	var runes [3]rune

	if err := gofret.ToInto("abc", &runes); err != nil {
		t.Fatal(err)
	}

	if runes != [3]rune{'a', 'b', 'c'} {
		t.Fatalf("got %#v", runes)
	}

	var wrong [3]int64

	strict := gofret.New(gofret.WithStrictTypes())

	if err := strict.ToInto("abc", &wrong); err == nil {
		t.Fatal("a string must not fill an int64 array in strict mode")
	}
}

func TestArrayFromSingleValueWeak(t *testing.T) {
	var out [2]int

	c := gofret.New(gofret.WithWeakTypes())

	if err := c.ToInto(7, &out); err != nil {
		t.Fatal(err)
	}

	if out != [2]int{7, 0} {
		t.Fatalf("got %#v", out)
	}
}

func TestArrayFromEmptyMapWeak(t *testing.T) {
	out := [2]int{1, 2}

	c := gofret.New(gofret.WithWeakTypes())

	if err := c.ToInto(map[string]any{}, &out); err != nil {
		t.Fatal(err)
	}

	if out != [2]int{} {
		t.Fatalf("got %#v", out)
	}
}

func TestUnexportedFieldsAreIgnored(t *testing.T) {
	type payload struct {
		Name   string `cfg:"name"`
		hidden string
	}

	got, err := gofret.To[map[string]any](payload{Name: "n", hidden: "h"})
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(got, map[string]any{"name": "n"}) {
		t.Fatalf("got %#v", got)
	}
}
