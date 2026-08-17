package gofret

// Option configures a Codec. Options are applied in order by New.
type Option func(*config)

// config is the resolved, immutable settings of a Codec.
type config struct {
	tag         string
	tagFallback []string

	hooks []Hook

	keyFunc    KeyFunc
	normalizer KeyNormalizer

	weakTypes      bool
	zeroFields     bool
	inlineEmbedded bool
	taggedOnly     bool
	derefPointers  bool
	omitNil        bool
	shallow        bool
	errorUnused    bool
	failFast       bool

	maxErrors int
}

// DefaultTag is the struct tag gofret reads when WithTag is not given.
const DefaultTag = "gofret"

func defaultConfig() config {
	return config{
		tag: DefaultTag,
		// Case-insensitive matching is the least surprising default for
		// configuration data, where key casing rarely matches Go field names.
		normalizer: FoldKey,
	}
}

// WithTag sets the struct tag to read. The default is DefaultTag.
func WithTag(tag string) Option {
	return func(c *config) { c.tag = tag }
}

// WithTagFallback adds tags consulted, in order, when the primary tag is
// absent on a field. It is handy for reusing existing `json` tags.
func WithTagFallback(tags ...string) Option {
	return func(c *config) { c.tagFallback = append(c.tagFallback, tags...) }
}

// WithHooks appends conversion hooks. They run in order before the built-in
// conversion; the first one that does not return ErrSkip wins.
func WithHooks(hooks ...Hook) Option {
	return func(c *config) { c.hooks = append(c.hooks, hooks...) }
}

// WithKeyFunc sets how a map key is derived from a field name that carries no
// name in its struct tag. See CamelCase, SnakeCase, KebabCase and PascalCase.
func WithKeyFunc(fn KeyFunc) Option {
	return func(c *config) { c.keyFunc = fn }
}

// WithKeyNormalizer replaces the fallback key matcher. Keys match when their
// normalized forms are equal. Passing nil disables fallback matching, making
// lookups exact.
//
// The default is FoldKey, which matches case-insensitively.
func WithKeyNormalizer(fn KeyNormalizer) Option {
	return func(c *config) { c.normalizer = fn }
}

// WithLooseKeys matches keys ignoring case, '-', '_' and ' ', so "MaxRetry",
// "max_retry" and "max-retry" all refer to the same field.
//
// It is shorthand for WithKeyNormalizer(LooseKey).
func WithLooseKeys() Option {
	return func(c *config) { c.normalizer = LooseKey }
}

// WithWeakTypes enables lenient scalar conversions:
//
//   - bool to and from string ("1"/"0", "true"/"false")
//   - number to and from string
//   - bool to and from number (true == 1)
//   - []byte and []rune to and from string
//   - a single value lifted into a one-element slice
//   - an empty map to an empty slice, and the reverse
//
// Without it, only conversions that cannot lose information are performed.
func WithWeakTypes() Option {
	return func(c *config) { c.weakTypes = true }
}

// WithZeroFields zeroes a destination before writing to it. Without it, maps
// and slices are merged into rather than replaced.
func WithZeroFields() Option {
	return func(c *config) { c.zeroFields = true }
}

// WithInlineEmbedded treats every embedded struct as if it carried the
// `inline` tag option.
func WithInlineEmbedded() Option {
	return func(c *config) { c.inlineEmbedded = true }
}

// WithTaggedOnly ignores fields that carry no struct tag, as if each of them
// were tagged "-".
func WithTaggedOnly() Option {
	return func(c *config) { c.taggedOnly = true }
}

// WithDerefPointers treats every pointer field as if it carried the `deref`
// tag option when writing to a map.
func WithDerefPointers() Option {
	return func(c *config) { c.derefPointers = true }
}

// WithOmitNil drops nil pointer, map, slice and interface fields when writing
// to a map.
func WithOmitNil() Option {
	return func(c *config) { c.omitNil = true }
}

// WithShallow treats every field as if it carried the `keep` tag option when
// writing to a map, so nested structs are passed through as-is instead of
// being converted recursively.
func WithShallow() Option {
	return func(c *config) { c.shallow = true }
}

// WithErrorUnused reports an error when the input holds keys that match no
// destination field and no `remain` field is present.
func WithErrorUnused() Option {
	return func(c *config) { c.errorUnused = true }
}

// WithFailFast stops at the first error. By default every error is collected
// and returned joined together.
func WithFailFast() Option {
	return func(c *config) { c.failFast = true }
}

// WithMaxErrors caps how many errors are collected before conversion stops.
// A value of zero or less means no cap.
func WithMaxErrors(n int) Option {
	return func(c *config) { c.maxErrors = n }
}
