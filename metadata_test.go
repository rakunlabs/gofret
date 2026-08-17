package gofret_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/rakunlabs/gofret"
)

func TestMetadata(t *testing.T) {
	type sub struct {
		Port int `gofret:"port"`
	}

	type payload struct {
		Name    string `gofret:"name"`
		Missing string `gofret:"missing"`
		Sub     sub    `gofret:"sub"`
	}

	in := map[string]any{
		"name": "n",
		"sub":  map[string]any{"port": 1},
		"typo": true,
	}

	var (
		md  gofret.Metadata
		out payload
	)

	if err := gofret.New().ToIntoMeta(in, &out, &md); err != nil {
		t.Fatal(err)
	}

	slices.Sort(md.Keys)
	slices.Sort(md.Unused)
	slices.Sort(md.Unset)

	if want := []string{"name", "sub", "sub.port"}; !reflect.DeepEqual(md.Keys, want) {
		t.Errorf("Keys = %q, want %q", md.Keys, want)
	}

	if want := []string{"typo"}; !reflect.DeepEqual(md.Unused, want) {
		t.Errorf("Unused = %q, want %q", md.Unused, want)
	}

	if !slices.Contains(md.Unset, "missing") {
		t.Errorf("Unset = %q, want it to mention \"missing\"", md.Unset)
	}
}

func TestMetadataNestedUnused(t *testing.T) {
	type sub struct {
		Port int `gofret:"port"`
	}

	type payload struct {
		Sub sub `gofret:"sub"`
	}

	in := map[string]any{"sub": map[string]any{"port": 1, "extra": 2}}

	var (
		md  gofret.Metadata
		out payload
	)

	if err := gofret.New().ToIntoMeta(in, &out, &md); err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(md.Unused, "sub.extra") {
		t.Fatalf("Unused = %q, want it to mention \"sub.extra\"", md.Unused)
	}
}

func TestMetadataReset(t *testing.T) {
	type payload struct {
		Name string `gofret:"name"`
	}

	var md gofret.Metadata

	c := gofret.New()

	for range 2 {
		md.Reset()

		var out payload

		if err := c.ToIntoMeta(map[string]any{"name": "n"}, &out, &md); err != nil {
			t.Fatal(err)
		}
	}

	if len(md.Keys) != 1 {
		t.Fatalf("Keys = %q, want one entry after Reset", md.Keys)
	}
}

func TestMetadataIsOptional(t *testing.T) {
	type payload struct {
		Name string `gofret:"name"`
	}

	var out payload

	// A nil Metadata must be accepted and cost nothing.
	if err := gofret.New().ToIntoMeta(map[string]any{"name": "n"}, &out, nil); err != nil {
		t.Fatal(err)
	}

	if out.Name != "n" {
		t.Fatalf("got %#v", out)
	}
}
