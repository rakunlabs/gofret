package gofret

import (
	"errors"
	"testing"
)

func TestParseTag(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    Tag
		wantErr bool
	}{
		{name: "empty", raw: "", want: Tag{}},
		{name: "skip", raw: "-", want: Tag{Opts: OptSkip}},
		{name: "name only", raw: "host", want: Tag{Name: "host"}},
		{name: "name with dash inside", raw: "a-b", want: Tag{Name: "a-b"}},
		{
			name: "options only",
			raw:  ",omitempty",
			want: Tag{Opts: OptOmitEmpty},
		},
		{
			name: "name and options",
			raw:  "host,omitempty,string",
			want: Tag{Name: "host", Opts: OptOmitEmpty | OptString},
		},
		{
			name: "every option",
			raw:  "x,omitempty,inline,remain,string,deref,keep",
			want: Tag{
				Name: "x",
				Opts: OptOmitEmpty | OptInline | OptRemain | OptString | OptDeref | OptKeep,
			},
		},
		{
			name: "repeated option is idempotent",
			raw:  "x,keep,keep",
			want: Tag{Name: "x", Opts: OptKeep},
		},
		{
			name: "empty option is ignored",
			raw:  "x,,keep",
			want: Tag{Name: "x", Opts: OptKeep},
		},
		{
			// A dash only means "skip" when it is the whole tag, so a field
			// can still be named "-something".
			name: "dash as a name",
			raw:  "-,omitempty",
			want: Tag{Name: "-", Opts: OptOmitEmpty},
		},
		{name: "unknown option", raw: "x,omitemty", wantErr: true},
		{name: "unknown option among valid", raw: "x,keep,nope", wantErr: true},
		{name: "squash is not our vocabulary", raw: "x,squash", wantErr: true},
		{name: "ptr2 is not our vocabulary", raw: "x,ptr2", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTag(tt.raw)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseTag(%q) = %+v, want an error", tt.raw, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseTag(%q): %v", tt.raw, err)
			}

			if got != tt.want {
				t.Fatalf("parseTag(%q) = %+v, want %+v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestTagHas(t *testing.T) {
	tag := Tag{Opts: OptOmitEmpty | OptKeep}

	if !tag.Has(OptOmitEmpty) {
		t.Error("Has(OptOmitEmpty) = false")
	}

	if !tag.Has(OptOmitEmpty | OptKeep) {
		t.Error("Has of both bits = false")
	}

	if tag.Has(OptOmitEmpty | OptString) {
		t.Error("Has must require every bit")
	}

	if !tag.HasAny(OptOmitEmpty | OptString) {
		t.Error("HasAny(one present) = false")
	}

	if tag.HasAny(OptString | OptDeref) {
		t.Error("HasAny(none present) = true")
	}
}

func TestOptString(t *testing.T) {
	if got := (OptOmitEmpty | OptKeep).String(); got != "omitempty,keep" {
		t.Fatalf("String() = %q", got)
	}

	if got := Opt(0).String(); got != "" {
		t.Fatalf("String() of no options = %q", got)
	}
}

func TestUnknownTagOptionSurfacesAsError(t *testing.T) {
	type bad struct {
		Name string `cfg:"name,omitemty"`
	}

	_, err := To[map[string]any](bad{Name: "x"})
	if err == nil {
		t.Fatal("a typo in a tag option must be reported, not ignored")
	}

	if !errors.Is(err, ErrInvalidTag) {
		t.Fatalf("err = %v, want it to wrap ErrInvalidTag", err)
	}
}
