package gofret_test

import (
	"reflect"
	"testing"

	"github.com/rakunlabs/gofret"
)

func TestSmokeMapToStruct(t *testing.T) {
	type Sub struct {
		Enabled bool `gofret:"enabled"`
	}

	type Config struct {
		Name  string   `gofret:"name"`
		Count int      `gofret:"count"`
		Hosts []string `gofret:"hosts"`
		Sub   Sub      `gofret:"sub"`
	}

	in := map[string]any{
		"name":  "altay",
		"count": "42",
		"hosts": []any{"a", "b"},
		"sub":   map[string]any{"enabled": "true"},
	}

	c := gofret.New(gofret.WithWeakTypes())

	got, err := c.To[Config](in)
	if err != nil {
		t.Fatalf("To: %v", err)
	}

	want := Config{
		Name:  "altay",
		Count: 42,
		Hosts: []string{"a", "b"},
		Sub:   Sub{Enabled: true},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestSmokeStructToMap(t *testing.T) {
	type Sub struct {
		Enabled bool `gofret:"enabled"`
	}

	type Config struct {
		Name  string   `gofret:"name"`
		Count int      `gofret:"count"`
		Hosts []string `gofret:"hosts"`
		Sub   Sub      `gofret:"sub"`
		Ptr   *Sub     `gofret:"ptr"`
	}

	in := Config{
		Name:  "altay",
		Count: 42,
		Hosts: []string{"a", "b"},
		Sub:   Sub{Enabled: true},
		Ptr:   &Sub{Enabled: false},
	}

	got, err := gofret.To[map[string]any](in)
	if err != nil {
		t.Fatalf("To: %v", err)
	}

	want := map[string]any{
		"name":  "altay",
		"count": 42,
		"hosts": []string{"a", "b"},
		"sub":   map[string]any{"enabled": true},
		"ptr":   map[string]any{"enabled": false},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %#v\nwant %#v", got, want)
	}
}

func TestSmokeRoundTrip(t *testing.T) {
	type Sub struct {
		Enabled bool   `gofret:"enabled"`
		Note    string `gofret:"note"`
	}

	type Config struct {
		Name  string   `gofret:"name"`
		Count int      `gofret:"count"`
		Hosts []string `gofret:"hosts"`
		Sub   Sub      `gofret:"sub"`
	}

	orig := Config{
		Name:  "altay",
		Count: 42,
		Hosts: []string{"a", "b"},
		Sub:   Sub{Enabled: true, Note: "hi"},
	}

	m, err := gofret.To[map[string]any](orig)
	if err != nil {
		t.Fatalf("to map: %v", err)
	}

	back, err := gofret.To[Config](m)
	if err != nil {
		t.Fatalf("to struct: %v", err)
	}

	if !reflect.DeepEqual(orig, back) {
		t.Fatalf("round trip changed the value:\n got %#v\nwant %#v", back, orig)
	}
}

func TestSmokeStructToStruct(t *testing.T) {
	type From struct {
		Name  string `gofret:"name"`
		Count int    `gofret:"count"`
	}

	type To struct {
		Name  string `gofret:"name"`
		Count int64  `gofret:"count"`
	}

	got, err := gofret.To[To](From{Name: "x", Count: 7})
	if err != nil {
		t.Fatalf("To: %v", err)
	}

	if got.Name != "x" || got.Count != 7 {
		t.Fatalf("got %#v", got)
	}
}
