package gofret_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rakunlabs/gofret"
)

func TestHookSkipFallsThrough(t *testing.T) {
	type payload struct {
		N int    `cfg:"n"`
		S string `cfg:"s"`
	}

	calls := 0

	c := gofret.New(gofret.WithHooks(func(ctx gofret.HookCtx) (any, error) {
		calls++

		return nil, gofret.ErrSkip
	}))

	got, err := c.To[payload](map[string]any{"n": 1, "s": "x"})
	if err != nil {
		t.Fatal(err)
	}

	if got.N != 1 || got.S != "x" {
		t.Fatalf("got %#v", got)
	}

	if calls == 0 {
		t.Fatal("the hook was never offered a value")
	}
}

// TestHookErrorPropagates pins down the central fix to the hook contract:
// the predecessor read any error as "decline", so a hook could not report a
// genuine failure at all.
func TestHookErrorPropagates(t *testing.T) {
	sentinel := errors.New("boom")

	c := gofret.New(gofret.WithHooks(func(ctx gofret.HookCtx) (any, error) {
		if ctx.To.Kind() == reflect.Int {
			return nil, sentinel
		}

		return nil, gofret.ErrSkip
	}))

	type payload struct {
		N int `cfg:"n"`
	}

	_, err := c.To[payload](map[string]any{"n": 1})
	if err == nil {
		t.Fatal("a failing hook must abort the conversion")
	}

	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want it to wrap the sentinel", err)
	}
}

func TestHookOrderFirstWins(t *testing.T) {
	first := gofret.HookBetween(func(s string) (string, error) { return "first:" + s, nil })
	second := gofret.HookBetween(func(s string) (string, error) { return "second:" + s, nil })

	c := gofret.New(gofret.WithHooks(first, second))

	got, err := c.To[string]("x")
	if err != nil {
		t.Fatal(err)
	}

	if got != "first:x" {
		t.Fatalf("got %q", got)
	}
}

// TestHookSeesFreshTypes covers the stale-type defect in the predecessor,
// where a chained hook was handed the type of the original input rather than
// the output of the hook before it.
func TestHookSeesFreshTypes(t *testing.T) {
	var seen []string

	record := gofret.Hook(func(ctx gofret.HookCtx) (any, error) {
		seen = append(seen, fmt.Sprintf("%s->%s", ctx.From, ctx.To))

		return nil, gofret.ErrSkip
	})

	type inner struct {
		N int `cfg:"n"`
	}

	type payload struct {
		Sub inner `cfg:"sub"`
	}

	c := gofret.New(gofret.WithHooks(record))

	if _, err := c.To[payload](map[string]any{"sub": map[string]any{"n": 1}}); err != nil {
		t.Fatal(err)
	}

	want := "int->int"
	if !slicesContains(seen, want) {
		t.Fatalf("expected a %q step among %q", want, seen)
	}
}

func slicesContains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}

	return false
}

func TestHookPathAndTag(t *testing.T) {
	type inner struct {
		Port int `cfg:"port,omitempty"`
	}

	type payload struct {
		Hosts []inner `cfg:"hosts"`
	}

	var got struct {
		path string
		tag  gofret.Tag
	}

	c := gofret.New(gofret.WithHooks(func(ctx gofret.HookCtx) (any, error) {
		if ctx.To.Kind() == reflect.Int {
			got.path = ctx.Path()
			got.tag = ctx.Tag
		}

		return nil, gofret.ErrSkip
	}))

	in := map[string]any{"hosts": []any{map[string]any{"port": 1}}}

	if _, err := c.To[payload](in); err != nil {
		t.Fatal(err)
	}

	if got.path != "hosts[0].port" {
		t.Fatalf("path = %q, want %q", got.path, "hosts[0].port")
	}

	if !got.tag.Has(gofret.OptOmitEmpty) {
		t.Fatalf("tag = %+v, want omitempty to be visible", got.tag)
	}
}

func TestHookTo(t *testing.T) {
	c := gofret.New(gofret.WithHooks(gofret.HookTo(func(in any) (time.Duration, error) {
		s, ok := in.(string)
		if !ok {
			return 0, gofret.ErrSkip
		}

		return time.ParseDuration(s)
	})))

	type payload struct {
		Timeout time.Duration `cfg:"timeout"`
	}

	got, err := c.To[payload](map[string]any{"timeout": "1h30m"})
	if err != nil {
		t.Fatal(err)
	}

	if got.Timeout != 90*time.Minute {
		t.Fatalf("got %v", got.Timeout)
	}
}

func TestHookFromMatchesOnSourceOnly(t *testing.T) {
	// Writing into a map the destination is the empty interface, so a hook
	// keyed on the destination type would never fire. HookFrom is the answer.
	c := gofret.New(gofret.WithHooks(gofret.HookFrom(func(t time.Time) (any, error) {
		return t.Format(time.RFC3339), nil
	})))

	type payload struct {
		At time.Time `cfg:"at"`
	}

	at := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

	got, err := c.To[map[string]any](payload{At: at})
	if err != nil {
		t.Fatal(err)
	}

	if got["at"] != "2026-08-17T12:00:00Z" {
		t.Fatalf("got %#v", got["at"])
	}
}

func TestHookBetween(t *testing.T) {
	c := gofret.New(gofret.WithHooks(gofret.HookBetween(func(s string) (time.Time, error) {
		return time.Parse("2006-01-02", s)
	})))

	type payload struct {
		Day time.Time `cfg:"day"`
	}

	got, err := c.To[payload](map[string]any{"day": "2026-08-17"})
	if err != nil {
		t.Fatal(err)
	}

	if got.Day.Year() != 2026 || got.Day.Month() != time.August || got.Day.Day() != 17 {
		t.Fatalf("got %v", got.Day)
	}
}

func TestDurationHook(t *testing.T) {
	type payload struct {
		Timeout time.Duration `cfg:"timeout"`
		Retry   time.Duration `cfg:"retry"`
	}

	c := gofret.New(gofret.WithHooks(gofret.DurationHook))

	got, err := c.To[payload](map[string]any{"timeout": "1h30m", "retry": 500})
	if err != nil {
		t.Fatal(err)
	}

	if got.Timeout != 90*time.Minute {
		t.Fatalf("timeout = %v", got.Timeout)
	}

	// Numbers keep the plain integer meaning of time.Duration.
	if got.Retry != 500 {
		t.Fatalf("retry = %v", got.Retry)
	}

	// The way out needs no hook at all, because time.Duration is a Stringer.
	back, err := c.To[map[string]string](payload{Timeout: 90 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}

	if back["timeout"] != "1h30m0s" {
		t.Fatalf("back = %#v", back)
	}
}

func TestDurationHookReportsBadInput(t *testing.T) {
	type payload struct {
		Timeout time.Duration `cfg:"timeout"`
	}

	c := gofret.New(gofret.WithHooks(gofret.DurationHook))

	_, err := c.To[payload](map[string]any{"timeout": "not a duration"})
	if err == nil {
		t.Fatal("an unparseable duration must be reported")
	}
}

func TestTimeHook(t *testing.T) {
	type payload struct {
		Day time.Time `cfg:"day"`
	}

	c := gofret.New(gofret.WithHooks(gofret.TimeHook("2006-01-02", time.RFC3339)))

	for _, in := range []string{"2026-08-17", "2026-08-17T00:00:00Z"} {
		got, err := c.To[payload](map[string]any{"day": in})
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}

		if got.Day.Day() != 17 {
			t.Fatalf("%q: got %v", in, got.Day)
		}
	}

	if _, err := c.To[payload](map[string]any{"day": "nope"}); err == nil {
		t.Fatal("a value matching no layout must be reported")
	}
}

// ---------------------------------------------------------------------------
// interfaces
// ---------------------------------------------------------------------------

type upperString string

func (u upperString) EncodeValue() (any, error) {
	return strings.ToUpper(string(u)), nil
}

func (u *upperString) DecodeValue(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("want a string, got %T", v)
	}

	*u = upperString(strings.ToLower(s))

	return nil
}

func TestValueEncoderAndDecoder(t *testing.T) {
	type payload struct {
		Name upperString `cfg:"name"`
	}

	m, err := gofret.To[map[string]any](payload{Name: "abc"})
	if err != nil {
		t.Fatal(err)
	}

	if m["name"] != "ABC" {
		t.Fatalf("EncodeValue was not used, got %#v", m["name"])
	}

	back, err := gofret.To[payload](map[string]any{"name": "XYZ"})
	if err != nil {
		t.Fatal(err)
	}

	if back.Name != "xyz" {
		t.Fatalf("DecodeValue was not used, got %q", back.Name)
	}
}

func TestValueDecoderErrorPropagates(t *testing.T) {
	type payload struct {
		Name upperString `cfg:"name"`
	}

	_, err := gofret.To[payload](map[string]any{"name": 42})
	if err == nil {
		t.Fatal("a failing DecodeValue must be reported")
	}
}

func TestHooksBeatInterfaces(t *testing.T) {
	// An explicit hook is the more specific instruction, so it wins over the
	// type's own opinion.
	c := gofret.New(gofret.WithHooks(gofret.HookFrom(func(u upperString) (any, error) {
		return "hooked", nil
	})))

	type payload struct {
		Name upperString `cfg:"name"`
	}

	m, err := c.To[map[string]any](payload{Name: "abc"})
	if err != nil {
		t.Fatal(err)
	}

	if m["name"] != "hooked" {
		t.Fatalf("got %#v", m["name"])
	}
}

// ---------------------------------------------------------------------------
// encoding.TextMarshaler / TextUnmarshaler
// ---------------------------------------------------------------------------

func TestTextMarshalerIsSymmetric(t *testing.T) {
	type payload struct {
		At time.Time `cfg:"at"`
	}

	at := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

	// time.Time is a TextMarshaler, so a string destination gets RFC 3339.
	m, err := gofret.To[map[string]string](payload{At: at})
	if err != nil {
		t.Fatal(err)
	}

	if m["at"] != "2026-08-17T12:00:00Z" {
		t.Fatalf("got %#v", m)
	}

	// It is a TextUnmarshaler too, so the same text reads straight back in
	// with no hook at all.
	back, err := gofret.To[payload](map[string]any{"at": "2026-08-17T12:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}

	if !back.At.Equal(at) {
		t.Fatalf("got %v, want %v", back.At, at)
	}
}

func TestTimeStaysWholeInAMap(t *testing.T) {
	type payload struct {
		At time.Time `cfg:"at"`
	}

	at := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

	// A struct with no reachable fields carries no map form, so it travels
	// intact rather than being exploded into wall/ext/loc.
	m, err := gofret.To[map[string]any](payload{At: at})
	if err != nil {
		t.Fatal(err)
	}

	got, ok := m["at"].(time.Time)
	if !ok {
		t.Fatalf("at = %#v, want a time.Time", m["at"])
	}

	if !got.Equal(at) {
		t.Fatalf("got %v, want %v", got, at)
	}
}
