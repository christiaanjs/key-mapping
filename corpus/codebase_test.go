package corpus

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const sampleGoFile = `package sample

// calculateTotalPrice adds up the shipping cost and returns the grand total.
// this is a simple helper used across the checkout flow.
func calculateTotalPrice(base_price int, shippingCost int) int {
	total_amount := base_price + shippingCost
	return total_amount
}

// HTTPServerConfig holds the server configuration.
type HTTPServerConfig struct {
	Port int
}
`

func TestFromCodebase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	if err := os.WriteFile(path, []byte(sampleGoFile), 0o644); err != nil {
		t.Fatalf("setup: writing sample file: %v", err)
	}
	// A non-matching extension file should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignoreme completely please"), 0o644); err != nil {
		t.Fatalf("setup: writing notes file: %v", err)
	}

	src, err := FromCodebase(dir, nil)
	if err != nil {
		t.Fatalf("FromCodebase error: %v", err)
	}

	wantSome := []string{"calculate", "total", "price", "base", "shipping", "cost", "amount", "http", "server", "config", "port"}
	got := make(map[string]bool)
	for _, w := range src.words {
		got[w] = true
		if w != strings.ToLower(w) {
			t.Errorf("word %q is not lowercase", w)
		}
	}
	for _, w := range wantSome {
		if !got[w] {
			t.Errorf("expected word %q among extracted words %v", w, src.words)
		}
	}

	if got["ignoreme"] {
		t.Errorf("words from non-matching extension file should not be included")
	}

	if len(src.sentences) == 0 {
		t.Errorf("expected at least one sentence extracted from comments")
	}
}

func TestFromCodebaseMissingRoot(t *testing.T) {
	_, err := FromCodebase(filepath.Join(t.TempDir(), "does-not-exist"), nil)
	if err == nil {
		t.Fatalf("expected error for missing root")
	}
}

func TestFromCodebaseNoMatches(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("nothing here matches"), 0o644); err != nil {
		t.Fatalf("setup: writing file: %v", err)
	}
	_, err := FromCodebase(dir, []string{".go"})
	if err == nil {
		t.Fatalf("expected error when no files match the extension filter")
	}
}

func TestSplitCamel(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"myVariableName", []string{"my", "Variable", "Name"}},
		{"HTTPServer", []string{"HTTP", "Server"}},
		{"simple", []string{"simple"}},
		{"ID", []string{"ID"}},
	}
	for _, tc := range cases {
		got := splitCamel([]rune(tc.in))
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitCamel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestIdentifierWords(t *testing.T) {
	got := identifierWords("myVariableName base_price HTTPServerConfig item2count")
	want := []string{"my", "variable", "name", "base", "price", "http", "server", "config", "item", "count"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("identifierWords = %v, want %v", got, want)
	}
}
