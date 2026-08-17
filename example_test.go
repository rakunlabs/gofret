package gofret_test

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rakunlabs/gofret"
)

func Example() {
	type Config struct {
		Name    string `cfg:"name"`
		Retries int    `cfg:"retries"`
	}

	cfg, err := gofret.To[Config](map[string]any{
		"name":    "service",
		"retries": 3,
	})
	if err != nil {
		fmt.Println(err)

		return
	}

	fmt.Printf("%s %d\n", cfg.Name, cfg.Retries)
	// Output:
	// service 3
}

// The destination type decides the direction, so writing a struct out is the
// same call with a different type argument.
func Example_toMap() {
	type Server struct {
		Host string `cfg:"host"`
		Port int    `cfg:"port"`
	}

	type Config struct {
		Name    string `cfg:"name"`
		Primary Server `cfg:"primary"`
	}

	m, err := gofret.To[map[string]any](Config{
		Name:    "service",
		Primary: Server{Host: "localhost", Port: 8080},
	})
	if err != nil {
		fmt.Println(err)

		return
	}

	printMap(m)
	// Output:
	// name: service
	// primary: map[host:localhost port:8080]
}

func Example_options() {
	type Config struct {
		MaxRetry int    `cfg:"maxRetry"`
		Name     string `json:"name"`
	}

	// The `cfg` tag is read by default; a fallback picks up the `json` tag on
	// fields that carry no `cfg` one.
	c := gofret.New(gofret.WithTagFallback("json"))

	// Weak typing accepts the string, and loose keys match "max_retry"
	// against "maxRetry". Both are on by default.
	cfg, err := c.To[Config](map[string]any{
		"max_retry": "5",
		"NAME":      "service",
	})
	if err != nil {
		fmt.Println(err)

		return
	}

	fmt.Printf("%d %s\n", cfg.MaxRetry, cfg.Name)
	// Output:
	// 5 service
}

func Example_hook() {
	type Config struct {
		Timeout time.Duration `cfg:"timeout"`
	}

	c := gofret.New(gofret.WithHooks(gofret.DurationHook))

	cfg, err := c.To[Config](map[string]any{"timeout": "1h30m"})
	if err != nil {
		fmt.Println(err)

		return
	}

	fmt.Println(cfg.Timeout)
	// Output:
	// 1h30m0s
}

// A hook declines with ErrSkip and fails with anything else, so a genuine
// problem is reported instead of quietly falling through.
func Example_hookError() {
	type Config struct {
		Port int `cfg:"port"`
	}

	c := gofret.New(gofret.WithHooks(func(ctx gofret.HookCtx) (any, error) {
		n, ok := ctx.Data().(int)
		if !ok {
			return nil, gofret.ErrSkip
		}

		if n < 1024 {
			return nil, fmt.Errorf("port %d is reserved", n)
		}

		return n, nil
	}))

	_, err := c.To[Config](map[string]any{"port": 80})

	fmt.Println(err)
	// Output:
	// gofret: port: cannot convert int to int: port 80 is reserved
}

// The `inline` option flattens a nested struct into its parent, in both
// directions.
func Example_inline() {
	type Auth struct {
		User string `cfg:"user"`
		Pass string `cfg:"pass"`
	}

	type Config struct {
		Host string `cfg:"host"`
		Auth Auth   `cfg:",inline"`
	}

	m, err := gofret.To[map[string]any](Config{
		Host: "db",
		Auth: Auth{User: "u", Pass: "p"},
	})
	if err != nil {
		fmt.Println(err)

		return
	}

	printMap(m)
	// Output:
	// host: db
	// pass: p
	// user: u
}

// The `remain` option collects the keys no field claimed, and writes them
// back out again, which is what keeps a round trip lossless.
func Example_remain() {
	type Config struct {
		Name string         `cfg:"name"`
		Rest map[string]any `cfg:",remain"`
	}

	cfg, err := gofret.To[Config](map[string]any{
		"name":    "service",
		"unknown": "kept",
	})
	if err != nil {
		fmt.Println(err)

		return
	}

	back, err := gofret.To[map[string]any](cfg)
	if err != nil {
		fmt.Println(err)

		return
	}

	printMap(back)
	// Output:
	// name: service
	// unknown: kept
}

func Example_error() {
	type Server struct {
		Port int `cfg:"port"`
	}

	type Config struct {
		Servers []Server `cfg:"servers"`
	}

	_, err := gofret.To[Config](map[string]any{
		"servers": []any{
			map[string]any{"port": 80},
			map[string]any{"port": "nope"},
		},
	})

	if ce, ok := errors.AsType[*gofret.Error](err); ok {
		fmt.Println("path:", ce.Path)
	}

	fmt.Println("unconvertible:", errors.Is(err, gofret.ErrUnconvertible))
	// Output:
	// path: servers[1].port
	// unconvertible: true
}

// A conversion reports every failure at once, so the joined error usually
// holds more than one. Errors lists them all.
func ExampleErrors() {
	type Config struct {
		Port    int `cfg:"port"`
		Retries int `cfg:"retries"`
	}

	_, err := gofret.To[Config](map[string]any{
		"port":    "nope",
		"retries": "many",
	})

	for _, ce := range gofret.Errors(err) {
		fmt.Printf("%s: %s -> %s\n", ce.Path, ce.From, ce.To)
	}
	// Output:
	// port: string -> int
	// retries: string -> int
}

func Example_metadata() {
	type Config struct {
		Name    string `cfg:"name"`
		Missing string `cfg:"missing"`
	}

	var (
		md  gofret.Metadata
		cfg Config
	)

	err := gofret.New().ToIntoMeta(map[string]any{
		"name": "service",
		"typo": true,
	}, &cfg, &md)
	if err != nil {
		fmt.Println(err)

		return
	}

	fmt.Println("used:  ", md.Keys)
	fmt.Println("unused:", md.Unused)
	fmt.Println("unset: ", md.Unset)
	// Output:
	// used:   [name]
	// unused: [typo]
	// unset:  [missing]
}

// A type can decide for itself how it is written and read.
func Example_valueEncoder() {
	type Config struct {
		Name csv `cfg:"name"`
	}

	m, err := gofret.To[map[string]any](Config{Name: csv{"a", "b"}})
	if err != nil {
		fmt.Println(err)

		return
	}

	printMap(m)

	back, err := gofret.To[Config](m)
	if err != nil {
		fmt.Println(err)

		return
	}

	fmt.Println(back.Name)
	// Output:
	// name: a,b
	// [a b]
}

type csv []string

func (c csv) EncodeValue() (any, error) { return strings.Join(c, ","), nil }

func (c *csv) DecodeValue(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("want a string, got %T", v)
	}

	*c = strings.Split(s, ",")

	return nil
}

// printMap renders a map with its keys in order, so the example output is
// stable.
func printMap(m map[string]any) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		fmt.Printf("%s: %v\n", k, m[k])
	}
}
