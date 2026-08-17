package gofret

import (
	"fmt"
	"strings"
)

// Tag holds the parsed struct tag of a single field.
//
// A tag looks like `gofret:"name,opt1,opt2"`. The first comma-separated
// element is the key name, the rest are options.
type Tag struct {
	// Name is the key name. Empty means "derive from the field name".
	Name string
	// Opts is the bitmask of the recognised options.
	Opts Opt
}

// Has reports whether opt is set. Multiple bits may be given, in which case
// it reports whether all of them are set.
func (t Tag) Has(opt Opt) bool { return t.Opts&opt == opt }

// HasAny reports whether at least one of the given bits is set.
func (t Tag) HasAny(opt Opt) bool { return t.Opts&opt != 0 }

// Opt is a bitmask of struct tag options.
type Opt uint16

const (
	// OptSkip is `-`: the field is ignored in both directions.
	OptSkip Opt = 1 << iota
	// OptOmitEmpty drops the field when its value is the zero value.
	// Applies when the destination is a map.
	OptOmitEmpty
	// OptInline merges the field's keys into the parent instead of nesting
	// them under the field name. Applies in both directions.
	OptInline
	// OptRemain collects keys that match no field. Applies in both
	// directions, which keeps round-trips lossless.
	OptRemain
	// OptString converts the value to its string form.
	OptString
	// OptDeref writes the pointed-to value instead of the pointer. A nil
	// pointer yields the zero value. Applies when the destination is a map.
	OptDeref
	// OptKeep passes the value through untouched instead of converting it
	// recursively. Applies when the destination is a map.
	OptKeep
)

// String renders the option names, comma separated, in declaration order.
func (o Opt) String() string {
	if o == 0 {
		return ""
	}

	var sb strings.Builder

	for _, n := range optNames {
		if o&n.opt == 0 {
			continue
		}

		if sb.Len() > 0 {
			sb.WriteByte(',')
		}

		sb.WriteString(n.name)
	}

	return sb.String()
}

type optName struct {
	name string
	opt  Opt
}

// optNames is the single source of truth for the tag vocabulary. Keep it in
// declaration order so Opt.String is deterministic.
var optNames = []optName{
	{"omitempty", OptOmitEmpty},
	{"inline", OptInline},
	{"remain", OptRemain},
	{"string", OptString},
	{"deref", OptDeref},
	{"keep", OptKeep},
}

// optByName is built from optNames so the two can never drift apart.
var optByName = func() map[string]Opt {
	m := make(map[string]Opt, len(optNames))
	for _, n := range optNames {
		m[n.name] = n.opt
	}

	return m
}()

// knownOpts lists the accepted option names for error messages.
var knownOpts = func() string {
	names := make([]string, 0, len(optNames))
	for _, n := range optNames {
		names = append(names, n.name)
	}

	return strings.Join(names, ", ")
}()

// parseTag parses a raw struct tag value.
//
// Unknown options are rejected rather than silently ignored, so a typo such as
// "omitemty" surfaces immediately instead of quietly changing behaviour. The
// cost is paid once per type because the result is cached in structInfo.
func parseTag(raw string) (Tag, error) {
	if raw == "-" {
		return Tag{Opts: OptSkip}, nil
	}

	name, rest, hasOpts := strings.Cut(raw, ",")

	tag := Tag{Name: name}
	if !hasOpts {
		return tag, nil
	}

	for rest != "" {
		var opt string

		opt, rest, _ = strings.Cut(rest, ",")
		if opt == "" {
			continue
		}

		bit, ok := optByName[opt]
		if !ok {
			return Tag{}, fmt.Errorf("unknown tag option %q (known options: %s)", opt, knownOpts)
		}

		tag.Opts |= bit
	}

	return tag, nil
}
