package gofret_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/rakunlabs/gofret"
)

// Struct-to-struct goes field to field using both type analyses. The
// predecessor routed it through an intermediate map, which lost information
// and made the result differ from a direct conversion.

func TestStructToStructRenamesByKey(t *testing.T) {
	type from struct {
		A string `gofret:"shared"`
		B string `gofret:"onlyfrom"`
	}

	type to struct {
		X string `gofret:"shared"`
	}

	got, err := gofret.To[to](from{A: "1", B: "2"})
	if err != nil {
		t.Fatal(err)
	}

	if got.X != "1" {
		t.Fatalf("got %#v", got)
	}
}

func TestStructToStructRemain(t *testing.T) {
	type from struct {
		Name  string `gofret:"name"`
		Extra int    `gofret:"extra"`
	}

	type to struct {
		Name string         `gofret:"name"`
		Rest map[string]any `gofret:",remain"`
	}

	got, err := gofret.To[to](from{Name: "n", Extra: 7})
	if err != nil {
		t.Fatal(err)
	}

	if got.Name != "n" {
		t.Fatalf("Name = %q", got.Name)
	}

	if !reflect.DeepEqual(got.Rest, map[string]any{"extra": 7}) {
		t.Fatalf("Rest = %#v", got.Rest)
	}
}

func TestStructToStructErrorUnused(t *testing.T) {
	type from struct {
		Name  string `gofret:"name"`
		Extra int    `gofret:"extra"`
	}

	type to struct {
		Name string `gofret:"name"`
	}

	if _, err := gofret.To[to](from{Name: "n", Extra: 7}); err != nil {
		t.Fatalf("unmatched fields are ignored by default: %v", err)
	}

	c := gofret.New(gofret.WithErrorUnused())

	_, err := c.To[to](from{Name: "n", Extra: 7})
	if !errors.Is(err, gofret.ErrUnusedKeys) {
		t.Fatalf("err = %v, want ErrUnusedKeys", err)
	}
}

func TestStructToStructMetadata(t *testing.T) {
	type from struct {
		Name  string `gofret:"name"`
		Extra int    `gofret:"extra"`
	}

	type to struct {
		Name    string `gofret:"name"`
		Missing string `gofret:"missing"`
	}

	var (
		md  gofret.Metadata
		out to
	)

	if err := gofret.New().ToIntoMeta(from{Name: "n", Extra: 7}, &out, &md); err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(md.Keys, "name") {
		t.Errorf("Keys = %q", md.Keys)
	}

	if !slices.Contains(md.Unused, "extra") {
		t.Errorf("Unused = %q", md.Unused)
	}

	if !slices.Contains(md.Unset, "missing") {
		t.Errorf("Unset = %q", md.Unset)
	}
}

func TestStructToStructOmitEmpty(t *testing.T) {
	type from struct {
		Name string `gofret:"name,omitempty"`
	}

	type to struct {
		Name string `gofret:"name"`
	}

	out := to{Name: "default"}

	// An empty source field is skipped, so the destination keeps its value.
	if err := gofret.ToInto(from{}, &out); err != nil {
		t.Fatal(err)
	}

	if out.Name != "default" {
		t.Fatalf("got %q, want the default to survive", out.Name)
	}
}

func TestStructToStructInlineOnBothSides(t *testing.T) {
	type base struct {
		Host string `gofret:"host"`
	}

	type from struct {
		Base base `gofret:",inline"`
	}

	type to struct {
		Host string `gofret:"host"`
	}

	got, err := gofret.To[to](from{Base: base{Host: "h"}})
	if err != nil {
		t.Fatal(err)
	}

	if got.Host != "h" {
		t.Fatalf("got %#v", got)
	}
}

func TestStructToStructConvertsFieldTypes(t *testing.T) {
	type from struct {
		N string `gofret:"n"`
	}

	type to struct {
		N int `gofret:"n"`
	}

	if _, err := gofret.To[to](from{N: "5"}); err == nil {
		t.Fatal("the strict codec must refuse string to int")
	}

	c := gofret.New(gofret.WithWeakTypes())

	got, err := c.To[to](from{N: "5"})
	if err != nil {
		t.Fatal(err)
	}

	if got.N != 5 {
		t.Fatalf("got %#v", got)
	}
}

func TestSameTypeIsCopiedDirectly(t *testing.T) {
	type payload struct {
		Name string `gofret:"name"`
	}

	orig := payload{Name: "n"}

	got, err := gofret.To[payload](orig)
	if err != nil {
		t.Fatal(err)
	}

	if got != orig {
		t.Fatalf("got %#v, want %#v", got, orig)
	}
}
