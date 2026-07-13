package corpus

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// defaultCodebaseExts is used by FromCodebase when the caller passes no
// extension filter.
var defaultCodebaseExts = []string{".go", ".js", ".ts", ".py", ".rs", ".java", ".c", ".h", ".cpp", ".rb"}

// FromCodebase walks root and builds a Source from source files whose
// extension is in exts (case-insensitive; leading dot optional). If exts is
// empty, defaultCodebaseExts is used.
//
// Words are extracted from identifiers: each file's contents are split into
// runs of ASCII letters, each run is further split on camelCase boundaries
// and underscores (underscores act as separators naturally since they are
// not letters), then lowercased and deduplicated.
//
// Sentences are extracted the same way as Sentences: lines with at least 4
// a-z words become practice sentences (useful for comments/docstrings).
//
// Unreadable files are skipped rather than failing the whole walk. An error
// is returned only if the walk itself fails (e.g. root does not exist) or if
// no usable words or sentences were found.
func FromCodebase(root string, exts []string) (*Source, error) {
	extSet := make(map[string]bool)
	if len(exts) == 0 {
		exts = defaultCodebaseExts
	}
	for _, e := range exts {
		e = strings.ToLower(e)
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		extSet[e] = true
	}

	wordSeen := make(map[string]bool)
	var words []string
	sentenceSeen := make(map[string]bool)
	var sentences []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !extSet[ext] {
			return nil
		}

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			// Skip unreadable files rather than failing the whole walk.
			return nil
		}
		content := string(data)

		for _, w := range identifierWords(content) {
			if !wordSeen[w] {
				wordSeen[w] = true
				words = append(words, w)
			}
		}
		for _, s := range Sentences(content) {
			if !sentenceSeen[s] {
				sentenceSeen[s] = true
				sentences = append(sentences, s)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(words) == 0 && len(sentences) == 0 {
		return nil, fmt.Errorf("corpus: no usable words or sentences found under %s", root)
	}

	return &Source{words: words, sentences: sentences}, nil
}

// identifierWords extracts lowercase word pieces from source code text: it
// finds maximal runs of ASCII letters (so digits, underscores, and any other
// punctuation act as separators), splits each run on camelCase boundaries,
// lowercases the pieces, and deduplicates preserving first-seen order.
func identifierWords(text string) []string {
	seen := make(map[string]bool)
	var words []string

	var run []rune
	flush := func() {
		if len(run) == 0 {
			return
		}
		for _, piece := range splitCamel(run) {
			w := strings.ToLower(piece)
			if isLowerAlpha(w) && !seen[w] {
				seen[w] = true
				words = append(words, w)
			}
		}
		run = run[:0]
	}

	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			run = append(run, r)
		} else {
			flush()
		}
	}
	flush()

	return words
}

// splitCamel splits a run of ASCII letters on camelCase boundaries, e.g.
// "myVariableName" -> ["my", "Variable", "Name"] and "HTTPServer" ->
// ["HTTP", "Server"].
func splitCamel(run []rune) []string {
	var parts []string
	var b []rune

	for i, r := range run {
		if i > 0 {
			prev := run[i-1]
			boundary := unicode.IsUpper(r) && (unicode.IsLower(prev) ||
				(unicode.IsUpper(prev) && i+1 < len(run) && unicode.IsLower(run[i+1])))
			if boundary && len(b) > 0 {
				parts = append(parts, string(b))
				b = b[:0]
			}
		}
		b = append(b, r)
	}
	if len(b) > 0 {
		parts = append(parts, string(b))
	}

	return parts
}
