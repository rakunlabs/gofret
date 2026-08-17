package gofret_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/rakunlabs/gofret"
)

func TestScalarConversions(t *testing.T) {
	tests := []struct {
		name    string
		weak    bool
		in      any
		out     any
		want    any
		wantErr bool
	}{
		// int family
		{name: "int to int", in: 5, out: new(int), want: 5},
		{name: "int to int64", in: 5, out: new(int64), want: int64(5)},
		{name: "uint to int", in: uint(5), out: new(int), want: 5},
		{name: "whole float to int", in: 5.0, out: new(int), want: 5},
		{name: "fractional float to int", in: 5.5, out: new(int), wantErr: true},
		{name: "fractional float to int weak", weak: true, in: 5.5, out: new(int), want: 5},
		{name: "int8 overflow", in: 5000, out: new(int8), wantErr: true},
		{name: "negative to uint", in: -1, out: new(uint), wantErr: true},
		{name: "string to int strict", in: "5", out: new(int), wantErr: true},
		{name: "string to int weak", weak: true, in: "5", out: new(int), want: 5},
		{name: "hex string to int weak", weak: true, in: "0x10", out: new(int), want: 16},
		{name: "empty string to int weak", weak: true, in: "", out: new(int), want: 0},
		{name: "bad string to int weak", weak: true, in: "nope", out: new(int), wantErr: true},
		{name: "bool to int weak", weak: true, in: true, out: new(int), want: 1},

		// json.Number is understood even in strict mode
		{name: "json number to int", in: json.Number("42"), out: new(int), want: 42},
		{name: "json number to float", in: json.Number("1.5"), out: new(float64), want: 1.5},

		// uint family
		{name: "uint to uint", in: uint(5), out: new(uint), want: uint(5)},
		{name: "uint8 overflow", in: 5000, out: new(uint8), wantErr: true},

		// float family
		{name: "int to float", in: 5, out: new(float64), want: 5.0},
		{name: "float32 overflow", in: 1e300, out: new(float32), wantErr: true},

		// bool
		{name: "bool to bool", in: true, out: new(bool), want: true},
		{name: "string to bool strict", in: "true", out: new(bool), wantErr: true},
		{name: "string to bool weak", weak: true, in: "true", out: new(bool), want: true},
		{name: "empty string to bool weak", weak: true, in: "", out: new(bool), want: false},
		{name: "bad string to bool weak", weak: true, in: "maybe", out: new(bool), wantErr: true},
		{name: "int to bool weak", weak: true, in: 3, out: new(bool), want: true},

		// string
		{name: "string to string", in: "x", out: new(string), want: "x"},
		{name: "int to string strict", in: 5, out: new(string), wantErr: true},
		{name: "int to string weak", weak: true, in: 5, out: new(string), want: "5"},
		{name: "bool to string weak", weak: true, in: true, out: new(string), want: "true"},
		{name: "bytes to string weak", weak: true, in: []byte("x"), out: new(string), want: "x"},

		// bytes and runes
		{name: "string to bytes", in: "abc", out: new([]byte), want: []byte("abc")},
		{name: "string to runes", in: "abc", out: new([]rune), want: []rune("abc")},

		// complex
		{name: "int to complex", in: 3, out: new(complex128), want: complex(3, 0)},

		// interface
		{name: "int to any", in: 5, out: new(any), want: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Weak typing is the default, so a case that does not ask for
			// it is exercising the strict codec.
			opts := []gofret.Option{gofret.WithStrictTypes()}
			if tt.weak {
				opts = []gofret.Option{gofret.WithWeakTypes()}
			}

			err := gofret.New(opts...).ToInto(tt.in, tt.out)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("want an error, got %#v", reflect.ValueOf(tt.out).Elem().Interface())
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			got := reflect.ValueOf(tt.out).Elem().Interface()
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v (%T), want %#v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestSliceConversions(t *testing.T) {
	t.Run("element conversion", func(t *testing.T) {
		var out []int

		if err := gofret.ToInto([]any{1, 2, 3}, &out); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(out, []int{1, 2, 3}) {
			t.Fatalf("got %#v", out)
		}
	})

	t.Run("nil source clears", func(t *testing.T) {
		out := []int{1}

		if err := gofret.ToInto([]any(nil), &out); err != nil {
			t.Fatal(err)
		}

		if out != nil {
			t.Fatalf("got %#v, want nil", out)
		}
	})

	t.Run("single value needs weak types", func(t *testing.T) {
		var out []int

		strict := gofret.New(gofret.WithStrictTypes())
		if err := strict.ToInto(4, &out); err == nil {
			t.Fatal("lifting a scalar into a slice must need weak types")
		}

		c := gofret.New(gofret.WithWeakTypes())
		if err := c.ToInto(4, &out); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(out, []int{4}) {
			t.Fatalf("got %#v", out)
		}
	})

	t.Run("empty map becomes an empty slice", func(t *testing.T) {
		var out []int

		c := gofret.New(gofret.WithWeakTypes())
		if err := c.ToInto(map[string]any{}, &out); err != nil {
			t.Fatal(err)
		}

		if out == nil || len(out) != 0 {
			t.Fatalf("got %#v, want an empty slice", out)
		}
	})

	t.Run("shorter source truncates", func(t *testing.T) {
		out := []int{9, 9, 9}

		if err := gofret.ToInto([]any{1}, &out); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(out, []int{1}) {
			t.Fatalf("got %#v", out)
		}
	})
}

func TestArrayConversions(t *testing.T) {
	t.Run("fits", func(t *testing.T) {
		var out [3]int

		if err := gofret.ToInto([]any{1, 2}, &out); err != nil {
			t.Fatal(err)
		}

		if out != [3]int{1, 2, 0} {
			t.Fatalf("got %#v", out)
		}
	})

	t.Run("too many values", func(t *testing.T) {
		var out [2]int

		if err := gofret.ToInto([]any{1, 2, 3}, &out); err == nil {
			t.Fatal("overflowing an array must be reported")
		}
	})

	t.Run("string to byte array", func(t *testing.T) {
		var out [3]byte

		if err := gofret.ToInto("abc", &out); err != nil {
			t.Fatal(err)
		}

		if out != [3]byte{'a', 'b', 'c'} {
			t.Fatalf("got %#v", out)
		}
	})
}

func TestMapConversions(t *testing.T) {
	t.Run("key conversion", func(t *testing.T) {
		var out map[int]string

		c := gofret.New(gofret.WithWeakTypes())
		if err := c.ToInto(map[string]any{"1": "a", "2": "b"}, &out); err != nil {
			t.Fatal(err)
		}

		want := map[int]string{1: "a", 2: "b"}
		if !reflect.DeepEqual(out, want) {
			t.Fatalf("got %#v, want %#v", out, want)
		}
	})

	t.Run("nil source clears", func(t *testing.T) {
		out := map[string]int{"a": 1}

		if err := gofret.ToInto(map[string]any(nil), &out); err != nil {
			t.Fatal(err)
		}

		if out != nil {
			t.Fatalf("got %#v, want nil", out)
		}
	})

	t.Run("slice of maps merges", func(t *testing.T) {
		var out map[string]int

		c := gofret.New(gofret.WithWeakTypes())

		in := []any{
			map[string]any{"a": 1},
			map[string]any{"b": 2},
		}

		if err := c.ToInto(in, &out); err != nil {
			t.Fatal(err)
		}

		want := map[string]int{"a": 1, "b": 2}
		if !reflect.DeepEqual(out, want) {
			t.Fatalf("got %#v, want %#v", out, want)
		}
	})
}

func TestPointerConversions(t *testing.T) {
	t.Run("allocates", func(t *testing.T) {
		var out *int

		if err := gofret.ToInto(5, &out); err != nil {
			t.Fatal(err)
		}

		if out == nil || *out != 5 {
			t.Fatalf("got %#v", out)
		}
	})

	t.Run("nil source clears", func(t *testing.T) {
		n := 5
		out := &n

		if err := gofret.ToInto((*int)(nil), &out); err != nil {
			t.Fatal(err)
		}

		if out != nil {
			t.Fatalf("got %#v, want nil", out)
		}
	})

	t.Run("source pointer is followed", func(t *testing.T) {
		n := 5

		var out int64

		if err := gofret.ToInto(&n, &out); err != nil {
			t.Fatal(err)
		}

		if out != 5 {
			t.Fatalf("got %#v", out)
		}
	})

	t.Run("double pointer", func(t *testing.T) {
		var out **int

		if err := gofret.ToInto(5, &out); err != nil {
			t.Fatal(err)
		}

		if out == nil || *out == nil || **out != 5 {
			t.Fatalf("got %#v", out)
		}
	})
}

func TestNonEmptyInterfaceDestination(t *testing.T) {
	type payload struct {
		S fmtStringer `cfg:"s"`
	}

	var out payload

	if err := gofret.ToInto(map[string]any{"s": level(2)}, &out); err != nil {
		t.Fatal(err)
	}

	if out.S.String() != "high" {
		t.Fatalf("got %q", out.S.String())
	}

	// A value that does not implement the interface is reported.
	if err := gofret.ToInto(map[string]any{"s": 5}, &out); err == nil {
		t.Fatal("assigning a non-implementing value must be reported")
	}
}

type fmtStringer interface{ String() string }

func TestNilHandling(t *testing.T) {
	type payload struct {
		Name string `cfg:"name"`
	}

	t.Run("nil value leaves the field alone", func(t *testing.T) {
		out := payload{Name: "kept"}

		if err := gofret.ToInto(map[string]any{"name": nil}, &out); err != nil {
			t.Fatal(err)
		}

		if out.Name != "kept" {
			t.Fatalf("got %q", out.Name)
		}
	})

	t.Run("nil value zeroes with WithZeroFields", func(t *testing.T) {
		out := payload{Name: "kept"}

		c := gofret.New(gofret.WithZeroFields())
		if err := c.ToInto(map[string]any{"name": nil}, &out); err != nil {
			t.Fatal(err)
		}

		if out.Name != "" {
			t.Fatalf("got %q", out.Name)
		}
	})
}
