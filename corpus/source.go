// Package corpus provides pluggable practice-text sources implementing the
// core.Corpus interface. Sources here live outside core because they need
// platform access (files, codebases, a local LLM over HTTP) that the pure
// core package must not depend on; they are intended for use by native
// frontends (e.g. the TUI), not the wasm web build.
package corpus

import "github.com/christiaanswanepoel/key-mapping/core"

// Source is a Corpus backed by an in-memory word and sentence bank. All
// sources in this package (file, codebase, ollama) build a *Source; the
// selection semantics below are copied verbatim from core's staticCorpus so
// behaviour is identical regardless of where the words came from.
type Source struct {
	words     []string
	sentences []string
}

// Word returns a practice word matching the length filter, chosen by seed.
// The selection is deterministic: same seed yields the same word.
func (s *Source) Word(length core.Length, seed int) string {
	pool := s.words

	switch length {
	case core.LengthShort:
		var filtered []string
		for _, w := range s.words {
			if len(w) <= 4 {
				filtered = append(filtered, w)
			}
		}
		if len(filtered) > 0 {
			pool = filtered
		}
	case core.LengthLong:
		var filtered []string
		for _, w := range s.words {
			if len(w) >= 6 {
				filtered = append(filtered, w)
			}
		}
		if len(filtered) > 0 {
			pool = filtered
		}
	}

	// Fall back to all words if pool is empty.
	if len(pool) == 0 {
		pool = s.words
	}

	if len(pool) == 0 {
		return ""
	}

	index := ((seed % len(pool)) + len(pool)) % len(pool)
	return pool[index]
}

// Sentence returns a practice sentence, chosen by seed.
// The selection is deterministic: same seed yields the same sentence.
func (s *Source) Sentence(seed int) string {
	if len(s.sentences) == 0 {
		return ""
	}
	index := ((seed % len(s.sentences)) + len(s.sentences)) % len(s.sentences)
	return s.sentences[index]
}

var _ core.Corpus = (*Source)(nil)
