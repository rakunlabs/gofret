// Package gofret converts Go values between shapes: maps into structs,
// structs into maps, and structs into other structs.
//
// # One entry point
//
// There is a single conversion function. What happens is decided by the
// destination type, not by which function you reach for:
//
//	cfg, err := gofret.To[Config](data)          // map    -> struct
//	m, err := gofret.To[map[string]any](cfg)     // struct -> map
//	err := gofret.ToInto(src, &dst)              // into an existing value
//
// Because both directions run through the same engine and the same analysis
// of each struct type, they cannot disagree about what a tag means. That is
// also what makes the round trip property hold: converting a value to a map
// and back returns the value unchanged.
//
// # Configuration
//
// The defaults suit configuration data, which is what gofret is mostly
// pointed at: keys match loosely, ignoring case, '-', '_' and ' ', and scalars
// convert leniently, so the "8080" that an environment variable hands you
// reaches an int field. [WithStrictKeys] and [WithStrictTypes] turn those off.
//
// Build a [Codec] when you want options. A Codec is immutable, safe for
// concurrent use, and caches its analysis of every struct type it sees, so
// make one and keep it:
//
//	c := gofret.New(
//	    gofret.WithTagFallback("json"),
//	    gofret.WithHooks(gofret.DurationHook),
//	    gofret.WithErrorUnused(),
//	)
//
// # Struct tags
//
// Fields are read from the `cfg` tag by default; see [WithTag]. The first
// element is the key name and the rest are options:
//
//	Field string `cfg:"name,omitempty"`
//
//	-           skip the field, in both directions
//	omitempty   drop the field when it holds the zero value
//	inline      merge the field's keys into the parent instead of nesting
//	remain      collect the keys that match no field, in both directions
//	string      carry the value as text
//	deref       write the pointed-to value; nil becomes the zero value
//	keep        pass the value through instead of converting it
//
// An unknown option is an error rather than something quietly ignored, so a
// typo such as "omitemty" surfaces the first time the type is used. The cost
// is paid once per type because the analysis is cached.
//
// # Hooks
//
// A [Hook] replaces a value before the built-in conversion runs. Return
// [ErrSkip] to decline and let the next hook, or the built-in conversion, take
// over. Any other error aborts the conversion and reaches the caller, so a
// hook can report a real failure instead of silently declining:
//
//	c := gofret.New(gofret.WithHooks(
//	    gofret.HookBetween(func(s string) (time.Time, error) {
//	        return time.Parse("2006-01-02", s)
//	    }),
//	))
//
// [HookTo], [HookFrom] and [HookBetween] build hooks from ordinary typed
// functions. A type can also speak for itself by implementing [ValueEncoder]
// or [ValueDecoder], and encoding.TextMarshaler, encoding.TextUnmarshaler and
// fmt.Stringer are honoured out of the box.
//
// # Errors
//
// Every failure is an [Error] carrying the dotted path of the value that
// caused it. Failures are collected and joined, so errors.Is and errors.AsType
// reach all of them.
//
// errors.AsType picks out the first one, and [Errors] lists them all:
//
//	if ce, ok := errors.AsType[*gofret.Error](err); ok {
//	    log.Printf("bad value at %s", ce.Path)
//	}
//
//	for _, ce := range gofret.Errors(err) {
//	    log.Printf("%s: %v", ce.Path, ce.Err)
//	}
//
// Failures are reported in destination field order rather than in the order
// Go happened to walk the input map, so the same input always reports the same
// thing. The same holds for [Metadata].
//
// See [WithFailFast] and [WithMaxErrors] to stop earlier, and
// [Codec.ToIntoMeta] to find out which keys went unused.
package gofret
