package gofret

import "testing"

func TestSplitWords(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"Name", []string{"Name"}},
		{"MaxRetry", []string{"Max", "Retry"}},
		{"maxRetry", []string{"max", "Retry"}},
		{"ID", []string{"ID"}},
		{"IDField", []string{"ID", "Field"}},
		{"HTTPServer", []string{"HTTP", "Server"}},
		{"max_retry", []string{"max", "retry"}},
		{"max-retry", []string{"max", "retry"}},
		{"max retry", []string{"max", "retry"}},
		{"sha256Sum", []string{"sha", "256", "Sum"}},
		{"__leading", []string{"leading"}},
		{"trailing__", []string{"trailing"}},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := splitWords(tt.in)

			if len(got) != len(tt.want) {
				t.Fatalf("splitWords(%q) = %q, want %q", tt.in, got, tt.want)
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("splitWords(%q) = %q, want %q", tt.in, got, tt.want)
				}
			}
		})
	}
}

func TestKeyFuncs(t *testing.T) {
	tests := []struct {
		in                                        string
		camel, pascal, snake, kebab, lower, loose string
	}{
		{
			in:    "MaxRetry",
			camel: "maxRetry", pascal: "MaxRetry", snake: "max_retry",
			kebab: "max-retry", lower: "maxretry", loose: "maxretry",
		},
		{
			in:    "ID",
			camel: "id", pascal: "Id", snake: "id",
			kebab: "id", lower: "id", loose: "id",
		},
		{
			in:    "HTTPServer",
			camel: "httpServer", pascal: "HttpServer", snake: "http_server",
			kebab: "http-server", lower: "httpserver", loose: "httpserver",
		},
		{
			in:    "max_retry",
			camel: "maxRetry", pascal: "MaxRetry", snake: "max_retry",
			kebab: "max-retry", lower: "max_retry", loose: "maxretry",
		},
		{
			in: "", camel: "", pascal: "", snake: "", kebab: "", lower: "", loose: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			check := func(name, got, want string) {
				t.Helper()

				if got != want {
					t.Errorf("%s(%q) = %q, want %q", name, tt.in, got, want)
				}
			}

			check("CamelCase", CamelCase(tt.in), tt.camel)
			check("PascalCase", PascalCase(tt.in), tt.pascal)
			check("SnakeCase", SnakeCase(tt.in), tt.snake)
			check("KebabCase", KebabCase(tt.in), tt.kebab)
			check("LowerCase", LowerCase(tt.in), tt.lower)
			check("LooseKey", LooseKey(tt.in), tt.loose)
		})
	}
}

func TestLooseKeyMatchesAcrossSeparators(t *testing.T) {
	forms := []string{"MaxRetry", "max_retry", "max-retry", "max retry", "MAXRETRY"}

	want := LooseKey(forms[0])
	for _, f := range forms[1:] {
		if got := LooseKey(f); got != want {
			t.Errorf("LooseKey(%q) = %q, want %q", f, got, want)
		}
	}
}
