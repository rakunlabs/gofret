package gofret_test

import (
	"reflect"
	"testing"

	"github.com/rakunlabs/gofret"
)

func TestWithTagAndFallback(t *testing.T) {
	type payload struct {
		A string `cfg:"a"`
		B string `json:"b"`
		C string `cfg:"c" json:"ignored"`
	}

	c := gofret.New(gofret.WithTag("cfg"), gofret.WithTagFallback("json"))

	got, err := c.To[map[string]any](payload{A: "1", B: "2", C: "3"})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]any{"a": "1", "b": "2", "c": "3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestWithTaggedOnlyIsSymmetric(t *testing.T) {
	type payload struct {
		Tagged   string `cfg:"tagged"`
		Untagged string
	}

	c := gofret.New(gofret.WithTag("cfg"), gofret.WithTaggedOnly())

	t.Run("to map", func(t *testing.T) {
		got, err := c.To[map[string]any](payload{Tagged: "a", Untagged: "b"})
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(got, map[string]any{"tagged": "a"}) {
			t.Fatalf("got %#v", got)
		}
	})

	// The predecessor applied this option in one direction only.
	t.Run("to struct", func(t *testing.T) {
		got, err := c.To[payload](map[string]any{"tagged": "a", "Untagged": "b"})
		if err != nil {
			t.Fatal(err)
		}

		if got.Untagged != "" {
			t.Fatalf("Untagged = %q, want it left alone", got.Untagged)
		}
	})
}

func TestWithKeyFunc(t *testing.T) {
	type payload struct {
		MaxRetry   int
		HTTPServer string
	}

	cases := []struct {
		name string
		fn   gofret.KeyFunc
		want map[string]any
	}{
		{"camel", gofret.CamelCase, map[string]any{"maxRetry": 1, "httpServer": "h"}},
		{"snake", gofret.SnakeCase, map[string]any{"max_retry": 1, "http_server": "h"}},
		{"kebab", gofret.KebabCase, map[string]any{"max-retry": 1, "http-server": "h"}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			c := gofret.New(gofret.WithKeyFunc(tt.fn))

			got, err := c.To[map[string]any](payload{MaxRetry: 1, HTTPServer: "h"})
			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestKeyMatching(t *testing.T) {
	type payload struct {
		MaxRetry int `cfg:"maxRetry"`
	}

	tests := []struct {
		name  string
		opts  []gofret.Option
		key   string
		match bool
	}{
		{"exact", nil, "maxRetry", true},
		{"case insensitive by default", nil, "MAXRETRY", true},
		{"separators need loose keys", nil, "max_retry", false},
		{"loose underscore", []gofret.Option{gofret.WithLooseKeys()}, "max_retry", true},
		{"loose dash", []gofret.Option{gofret.WithLooseKeys()}, "max-retry", true},
		{"loose space", []gofret.Option{gofret.WithLooseKeys()}, "max retry", true},
		{
			"normalizer off means exact only",
			[]gofret.Option{gofret.WithKeyNormalizer(nil)},
			"MAXRETRY",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := append([]gofret.Option{gofret.WithTag("cfg")}, tt.opts...)
			c := gofret.New(opts...)

			got, err := c.To[payload](map[string]any{tt.key: 7})
			if err != nil {
				t.Fatal(err)
			}

			if tt.match && got.MaxRetry != 7 {
				t.Fatalf("%q should have matched, got %#v", tt.key, got)
			}

			if !tt.match && got.MaxRetry != 0 {
				t.Fatalf("%q should not have matched, got %#v", tt.key, got)
			}
		})
	}
}

func TestWithWeakTypes(t *testing.T) {
	type payload struct {
		N int    `gofret:"n"`
		S string `gofret:"s"`
		B bool   `gofret:"b"`
	}

	in := map[string]any{"n": "42", "s": 7, "b": "true"}

	if _, err := gofret.To[payload](in); err == nil {
		t.Fatal("the strict codec must refuse these conversions")
	}

	c := gofret.New(gofret.WithWeakTypes())

	got, err := c.To[payload](in)
	if err != nil {
		t.Fatal(err)
	}

	want := payload{N: 42, S: "7", B: true}
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestWithOmitNil(t *testing.T) {
	type payload struct {
		Ptr   *int           `gofret:"ptr"`
		Slice []int          `gofret:"slice"`
		Map   map[string]int `gofret:"map"`
		Set   string         `gofret:"set"`
	}

	c := gofret.New(gofret.WithOmitNil())

	got, err := c.To[map[string]any](payload{Set: "x"})
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(got, map[string]any{"set": "x"}) {
		t.Fatalf("got %#v", got)
	}
}

func TestWithZeroFields(t *testing.T) {
	type payload struct {
		Tags map[string]string `gofret:"tags"`
	}

	in := map[string]any{"tags": map[string]any{"b": "2"}}

	t.Run("merges by default", func(t *testing.T) {
		out := payload{Tags: map[string]string{"a": "1"}}

		if err := gofret.ToInto(in, &out); err != nil {
			t.Fatal(err)
		}

		want := map[string]string{"a": "1", "b": "2"}
		if !reflect.DeepEqual(out.Tags, want) {
			t.Fatalf("got %#v, want %#v", out.Tags, want)
		}
	})

	t.Run("replaces when zeroing", func(t *testing.T) {
		out := payload{Tags: map[string]string{"a": "1"}}

		c := gofret.New(gofret.WithZeroFields())
		if err := c.ToInto(in, &out); err != nil {
			t.Fatal(err)
		}

		want := map[string]string{"b": "2"}
		if !reflect.DeepEqual(out.Tags, want) {
			t.Fatalf("got %#v, want %#v", out.Tags, want)
		}
	})
}

func TestPartialInputKeepsDefaults(t *testing.T) {
	type payload struct {
		Host string `gofret:"host"`
		Port int    `gofret:"port"`
	}

	out := payload{Host: "localhost", Port: 8080}

	if err := gofret.ToInto(map[string]any{"port": 9090}, &out); err != nil {
		t.Fatal(err)
	}

	if out.Host != "localhost" || out.Port != 9090 {
		t.Fatalf("got %#v", out)
	}
}

func TestCodecIsReusableAndConcurrent(t *testing.T) {
	type payload struct {
		N int `gofret:"n"`
	}

	c := gofret.New(gofret.WithWeakTypes())

	done := make(chan error, 8)

	for i := range 8 {
		go func() {
			got, err := c.To[payload](map[string]any{"n": i})
			if err == nil && got.N != i {
				t.Errorf("got %d, want %d", got.N, i)
			}

			done <- err
		}()
	}

	for range 8 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}
