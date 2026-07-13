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
	return selectWord(s.words, length, seed)
}

// Sentence returns a practice sentence, chosen by seed.
// The selection is deterministic: same seed yields the same sentence.
func (s *Source) Sentence(seed int) string {
	return selectSentence(s.sentences, seed)
}

var _ core.Corpus = (*Source)(nil)

// selectWord and selectSentence hold the length-filter/seed selection logic
// shared by every bank-backed Corpus in this package (Source and Stream), so
// they behave identically to core's staticCorpus regardless of where the
// words came from or whether the bank is still growing.

// selectWord returns a word from words matching the length filter, chosen by
// seed. Falls back to the unfiltered pool if the filter matches nothing.
func selectWord(words []string, length core.Length, seed int) string {
	pool := words

	switch length {
	case core.LengthShort:
		var filtered []string
		for _, w := range words {
			if len(w) <= 4 {
				filtered = append(filtered, w)
			}
		}
		if len(filtered) > 0 {
			pool = filtered
		}
	case core.LengthLong:
		var filtered []string
		for _, w := range words {
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
		pool = words
	}

	if len(pool) == 0 {
		return ""
	}

	index := ((seed % len(pool)) + len(pool)) % len(pool)
	return pool[index]
}

// selectSentence returns a sentence from sentences, chosen by seed.
func selectSentence(sentences []string, seed int) string {
	if len(sentences) == 0 {
		return ""
	}
	index := ((seed % len(sentences)) + len(sentences)) % len(sentences)
	return sentences[index]
}
