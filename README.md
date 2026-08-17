# gofret

[![License](https://img.shields.io/github/license/rakunlabs/gofret?color=red&style=flat-square)](https://raw.githubusercontent.com/rakunlabs/gofret/main/LICENSE)
[![CI](https://img.shields.io/github/actions/workflow/status/rakunlabs/gofret/test.yml?branch=main&logo=github&style=flat-square&label=ci)](https://github.com/rakunlabs/gofret/actions)
[![Go Reference](https://img.shields.io/badge/reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/rakunlabs/gofret)

Convert Go values between shapes: maps into structs, structs into maps, structs
into other structs. No dependencies.

```sh
go get github.com/rakunlabs/gofret
```

Requires Go 1.27.

## One entry point

There is a single conversion function. The destination type decides what
happens, so there is no separate encoder and decoder to keep in step:

```go
cfg, err := gofret.To[Config](data)          // map    -> struct
m,   err := gofret.To[map[string]any](cfg)   // struct -> map
err       = gofret.ToInto(src, &dst)         // into an existing value
```

Both directions run through the same engine and the same analysis of each
struct type, so they cannot disagree about what a tag means. That is also what
makes the round trip hold:

```go
m,    _ := gofret.To[map[string]any](cfg)
back, _ := gofret.To[Config](m)
// reflect.DeepEqual(cfg, back) == true
```

## Options

The package-level functions use a strict, zero-configuration codec. Build a
`Codec` when you want options. It is immutable, safe for concurrent use, and
caches its analysis of every struct type it sees, so make one and keep it:

```go
c := gofret.New(
    gofret.WithTag("cfg"),
    gofret.WithWeakTypes(),
    gofret.WithLooseKeys(),
    gofret.WithHooks(gofret.DurationHook),
)

err := c.ToInto(data, &cfg)
```

| option | effect |
| --- | --- |
| `WithTag(s)` | struct tag to read, default `gofret` |
| `WithTagFallback(s...)` | tags consulted when the primary one is absent |
| `WithHooks(h...)` | conversion hooks |
| `WithWeakTypes()` | lenient scalar conversions: `"42"` to `42`, `1` to `true` |
| `WithLooseKeys()` | match keys ignoring case, `-`, `_` and ` ` |
| `WithKeyNormalizer(fn)` | replace the fallback key matcher; `nil` means exact only |
| `WithKeyFunc(fn)` | derive keys from field names: `CamelCase`, `SnakeCase`, `KebabCase`, `PascalCase` |
| `WithZeroFields()` | zero a destination before writing instead of merging into it |
| `WithInlineEmbedded()` | treat every embedded struct as `inline` |
| `WithTaggedOnly()` | ignore fields with no struct tag |
| `WithDerefPointers()` | treat every pointer field as `deref` |
| `WithOmitNil()` | drop nil fields when writing to a map |
| `WithShallow()` | treat every field as `keep` |
| `WithErrorUnused()` | fail when the input holds keys no field claims |
| `WithFailFast()` | stop at the first error |
| `WithMaxErrors(n)` | cap how many errors are collected |

Keys match case-insensitively by default, which is usually what configuration
data needs. `WithKeyNormalizer(nil)` turns that off.

## Struct tags

```go
type Config struct {
    Name    string         `gofret:"name"`
    Secret  string         `gofret:"-"`
    Debug   bool           `gofret:"debug,omitempty"`
    Auth    Auth           `gofret:",inline"`
    Rest    map[string]any `gofret:",remain"`
}
```

| tag | to a map | to a struct |
| --- | :---: | :---: |
| `-` | skip | skip |
| `omitempty` | drop the zero value | |
| `inline` | merge into the parent | read from the parent |
| `remain` | write the captured keys back | collect the unclaimed keys |
| `string` | render as text | parse from text |
| `deref` | write the pointed-to value | |
| `keep` | pass through unconverted | |

An unknown option is an error rather than something quietly ignored, so a typo
such as `omitemty` surfaces the first time the type is used. The check runs
once per type, because the analysis is cached.

## Hooks

A hook replaces a value before the built-in conversion runs. Return `ErrSkip`
to decline; **any other error aborts the conversion and reaches the caller**,
so a hook can report a real problem instead of quietly falling through.

```go
c := gofret.New(gofret.WithHooks(
    gofret.HookBetween(func(s string) (time.Time, error) {
        return time.Parse("2006-01-02", s)
    }),
))
```

| builder | fires when |
| --- | --- |
| `HookTo[T](fn)` | the destination type is exactly `T` |
| `HookFrom[T](fn)` | the source is assignable to `T`, whatever the destination |
| `HookBetween[In, Out](fn)` | both |

Use `HookFrom` when writing into a map, where the destination is the empty
interface and carries no type information.

For full control, write the `Hook` yourself:

```go
func(ctx gofret.HookCtx) (any, error)
```

`HookCtx` gives you the source and destination types (`From`, `To`), the
`Value`, and the parsed `Tag` of the field being converted. `ctx.Data()` boxes
the value into an `any` and `ctx.Path()` builds the dotted location; both are
methods so a hook that only inspects types and declines allocates nothing.

Built-in hooks: `DurationHook`, `TimeHook(layouts...)`, `TimeFormatHook(layout)`,
`NilHook`.

### Types that speak for themselves

```go
type ValueEncoder interface{ EncodeValue() (any, error) }
type ValueDecoder interface{ DecodeValue(any) error }
```

`encoding.TextMarshaler`, `encoding.TextUnmarshaler` and `fmt.Stringer` are
honoured out of the box, so a `time.Time` field reads and writes RFC 3339 and a
`time.Duration` comes out as `"1h30m0s"` with no configuration at all.

## Errors

Every failure is a `*gofret.Error` carrying the dotted path of the offending
value. Failures are collected and joined, so `errors.Is` and `errors.AsType`
reach all of them.

```go
if ce, ok := errors.AsType[*gofret.Error](err); ok {
    log.Printf("bad value at %s", ce.Path) // "servers[1].port"
}
```

A conversion reports every failure at once, so the tree usually holds more
than one. `errors.AsType` gives you the first; `gofret.Errors` gives you all
of them:

```go
for _, ce := range gofret.Errors(err) {
    log.Printf("%s: %v", ce.Path, ce.Err)
}
```

Failures come back in destination field order, not in whatever order Go
happened to walk the input map, so the same input always reports the same
thing. The same holds for `Metadata`.

Sentinels: `ErrSkip`, `ErrNotPointer`, `ErrUnconvertible`, `ErrUnsupportedType`,
`ErrOverflow`, `ErrUnusedKeys`, `ErrInvalidTag`.

Nothing panics. A bad input, a bad destination or a bad tag all arrive as an
error.

## Metadata

```go
var md gofret.Metadata

err := c.ToIntoMeta(data, &cfg, &md)

md.Keys    // paths that were written
md.Unused  // input keys no field claimed, the usual way to warn about typos
md.Unset   // fields no input supplied
```

Nothing is tracked when you do not ask for it, so the default path stays cheap.

## Performance

Analysis of each struct type is cached per codec, tag options are a bitmask,
and key matching is precomputed, so no work is repeated per call.

Converting an eight-field config with two nested structs, on a Ryzen 7 5800X:

**map to struct**

| | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| gofret | **4.30µ** | **2.03Ki** | **59** |
| struct2 v1.4.0 | 10.12µ | 8.63Ki | 142 |
| mapstructure v2.2.1 | 10.51µ | 7.77Ki | 137 |

**struct to map**

| | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| gofret | **5.21µ** | **2.92Ki** | 72 |
| struct2 v1.4.0 | 6.75µ | 5.85Ki | 66 |

A configured hook that declines costs about 10% and no allocations, because
`HookCtx` builds the expensive parts only when asked.

Reuse a `Codec`; `New` starts with a cold cache.

## License

[BSD Zero Clause](./LICENSE)
