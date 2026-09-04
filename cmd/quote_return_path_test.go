package cmd

import (
	"strings"
	"testing"
)

func TestQuotePOSIXReturnPathNeutralizesShellMetacharacters(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "plain", path: "/pool/1/repo", want: "'/pool/1/repo'"},
		{name: "spaces", path: "/pool/my repo", want: "'/pool/my repo'"},
		{name: "double quotes", path: `/pool/say "hi"`, want: `'/pool/say "hi"'`},
		{name: "single quote", path: "/pool/it's", want: `'/pool/it'\''s'`},
		{name: "command substitution", path: "/pool/$(id)", want: "'/pool/$(id)'"},
		{name: "backticks", path: "/pool/`id`", want: "'/pool/`id`'"},
		{name: "separators", path: "/pool/a; rm -rf /", want: "'/pool/a; rm -rf /'"},
		{name: "pipe and and", path: "/pool/a|b&&c", want: "'/pool/a|b&&c'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := quotePOSIXReturnPath(tc.path)
			if got != tc.want {
				t.Fatalf("quotePOSIXReturnPath(%q) = %q, want %q", tc.path, got, tc.want)
			}
			if !strings.HasPrefix(got, "'") || !strings.HasSuffix(got, "'") {
				t.Fatalf("expected POSIX single quotes around %q, got %q", tc.path, got)
			}
		})
	}
}

func TestQuoteWindowsReturnPathDoublesInternalQuotes(t *testing.T) {
	t.Parallel()

	got := quoteWindowsReturnPath(`C:\pool\say "hi"`)
	want := `"C:\pool\say ""hi"""`
	if got != want {
		t.Fatalf("quoteWindowsReturnPath = %q, want %q", got, want)
	}
}

func TestQuoteWindowsReturnPathDoublesPercentSigns(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "env-like component", path: `C:\pool\%TARGET%\repo`, want: `"C:\pool\%%TARGET%%\repo"`},
		{name: "percent and quotes", path: `C:\pool\%A%\say "hi"`, want: `"C:\pool\%%A%%\say ""hi"""`},
		{name: "plain", path: `C:\pool\1\repo`, want: `"C:\pool\1\repo"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := quoteWindowsReturnPath(tc.path)
			if got != tc.want {
				t.Fatalf("quoteWindowsReturnPath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestQuoteReturnPathEmpty(t *testing.T) {
	t.Parallel()

	if got := quoteReturnPath(""); got != "" {
		t.Fatalf("quoteReturnPath empty = %q, want empty", got)
	}
}
