package corpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleText = `the quick brown fox jumps over the lazy dog
hi
a fine day for a walk in the park
The Quick Brown Fox Jumps Again Today`

func TestFromText(t *testing.T) {
	src := FromText(sampleText)

	if len(src.words) == 0 {
		t.Fatalf("expected words to be extracted")
	}
	if len(src.sentences) == 0 {
		t.Fatalf("expected sentences to be extracted")
	}

	foundFox := false
	for _, w := range src.words {
		if w == "fox" {
			foundFox = true
		}
	}
	if !foundFox {
		t.Errorf("expected %q among words, got %v", "fox", src.words)
	}
}

func TestFromReader(t *testing.T) {
	src, err := FromReader(strings.NewReader(sampleText))
	if err != nil {
		t.Fatalf("FromReader error: %v", err)
	}
	if len(src.words) == 0 || len(src.sentences) == 0 {
		t.Fatalf("expected non-empty words and sentences from reader")
	}
}

func TestFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "practice.txt")
	if err := os.WriteFile(path, []byte(sampleText), 0o644); err != nil {
		t.Fatalf("setup: writing temp file: %v", err)
	}

	src, err := FromFile(path)
	if err != nil {
		t.Fatalf("FromFile error: %v", err)
	}
	if len(src.words) == 0 || len(src.sentences) == 0 {
		t.Fatalf("expected non-empty words and sentences from file")
	}
}

func TestFromFileMissing(t *testing.T) {
	_, err := FromFile(filepath.Join(t.TempDir(), "does-not-exist.txt"))
	if err == nil {
		t.Fatalf("expected error reading missing file")
	}
}
