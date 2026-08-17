package gofret_test

import (
	"testing"

	"github.com/rakunlabs/gofret"
)

type benchSub struct {
	Host    string `cfg:"host"`
	Port    int    `cfg:"port"`
	Enabled bool   `cfg:"enabled"`
}

type benchConfig struct {
	Name    string            `cfg:"name"`
	Retries int               `cfg:"retries"`
	Ratio   float64           `cfg:"ratio"`
	Debug   bool              `cfg:"debug"`
	Tags    []string          `cfg:"tags"`
	Labels  map[string]string `cfg:"labels"`
	Primary benchSub          `cfg:"primary"`
	Nodes   []benchSub        `cfg:"nodes"`
}

func benchInput() map[string]any {
	return map[string]any{
		"name":    "service",
		"retries": 3,
		"ratio":   1.5,
		"debug":   true,
		"tags":    []any{"a", "b", "c"},
		"labels":  map[string]any{"env": "prod", "tier": "web"},
		"primary": map[string]any{"host": "h1", "port": 8080, "enabled": true},
		"nodes": []any{
			map[string]any{"host": "h2", "port": 8081, "enabled": false},
			map[string]any{"host": "h3", "port": 8082, "enabled": true},
		},
	}
}

func benchValue() benchConfig {
	return benchConfig{
		Name:    "service",
		Retries: 3,
		Ratio:   1.5,
		Debug:   true,
		Tags:    []string{"a", "b", "c"},
		Labels:  map[string]string{"env": "prod", "tier": "web"},
		Primary: benchSub{Host: "h1", Port: 8080, Enabled: true},
		Nodes: []benchSub{
			{Host: "h2", Port: 8081},
			{Host: "h3", Port: 8082, Enabled: true},
		},
	}
}

func BenchmarkMapToStruct(b *testing.B) {
	c := gofret.New()
	in := benchInput()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var out benchConfig

		if err := c.ToInto(in, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMapToStructWeak(b *testing.B) {
	c := gofret.New(gofret.WithWeakTypes(), gofret.WithLooseKeys())
	in := benchInput()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var out benchConfig

		if err := c.ToInto(in, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStructToMap(b *testing.B) {
	c := gofret.New()
	in := benchValue()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := c.To[map[string]any](in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStructToStruct(b *testing.B) {
	c := gofret.New()
	in := benchValue()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := c.To[benchConfig](in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRoundTrip(b *testing.B) {
	c := gofret.New()
	in := benchValue()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		m, err := c.To[map[string]any](in)
		if err != nil {
			b.Fatal(err)
		}

		if _, err := c.To[benchConfig](m); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStructInfoCacheMiss measures the analysis that the cache normally
// hides, by using a fresh codec every iteration.
func BenchmarkStructInfoCacheMiss(b *testing.B) {
	in := benchInput()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var out benchConfig

		if err := gofret.New().ToInto(in, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWithMetadata(b *testing.B) {
	c := gofret.New()
	in := benchInput()

	var md gofret.Metadata

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		md.Reset()

		var out benchConfig

		if err := c.ToIntoMeta(in, &out, &md); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWithHook(b *testing.B) {
	c := gofret.New(gofret.WithHooks(gofret.DurationHook))
	in := benchInput()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var out benchConfig

		if err := c.ToInto(in, &out); err != nil {
			b.Fatal(err)
		}
	}
}

// The benchmarks below justify collecting every error rather than stopping at
// the first: on input that converts cleanly the bookkeeping costs nothing, and
// the extra work only shows up on input that was going to be rejected anyway.

type errBench struct {
	A int `cfg:"a"`
	B int `cfg:"b"`
	C int `cfg:"c"`
	D int `cfg:"d"`
	E int `cfg:"e"`
	F int `cfg:"f"`
	G int `cfg:"g"`
	H int `cfg:"h"`
}

func runErrBench(b *testing.B, c *gofret.Codec, in map[string]any, wantErr bool) {
	b.Helper()
	b.ReportAllocs()

	for b.Loop() {
		var out errBench

		err := c.ToInto(in, &out)
		if !wantErr && err != nil {
			b.Fatal(err)
		}
	}
}

func cleanErrInput() map[string]any {
	return map[string]any{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6, "g": 7, "h": 8}
}

func brokenErrInput() map[string]any {
	return map[string]any{"a": "x", "b": "x", "c": "x", "d": "x", "e": "x", "f": "x", "g": "x", "h": "x"}
}

func BenchmarkCleanInputCollecting(b *testing.B) {
	runErrBench(b, gofret.New(), cleanErrInput(), false)
}

func BenchmarkCleanInputFailFast(b *testing.B) {
	runErrBench(b, gofret.New(gofret.WithFailFast()), cleanErrInput(), false)
}

func BenchmarkBrokenInputCollecting(b *testing.B) {
	runErrBench(b, gofret.New(), brokenErrInput(), true)
}

func BenchmarkBrokenInputFailFast(b *testing.B) {
	runErrBench(b, gofret.New(gofret.WithFailFast()), brokenErrInput(), true)
}
