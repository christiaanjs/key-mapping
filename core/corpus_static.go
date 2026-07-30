package core

import "strings"

// isLowerAlpha reports whether s is non-empty and all ASCII a-z. Used in place
// of a ^[a-z]+$ regexp so the pure core does not pull the regexp package into
// the wasm binary (binary size is a concern for the web target).
func isLowerAlpha(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// staticCorpus is a TEMPORARY hardcoded bank of words and sentences copied from
// the prototype. Later phases will replace this with codebase extraction or
// LLM generation behind the same Corpus interface.
type staticCorpus struct {
	words     []string
	sentences []string
}

// Word returns a practice word matching the length filter, chosen by seed.
// The selection is deterministic: same seed yields the same word.
func (c *staticCorpus) Word(length Length, seed int) string {
	pool := c.words

	// Filter by length
	switch length {
	case LengthShort:
		var filtered []string
		for _, w := range c.words {
			if len(w) <= 4 {
				filtered = append(filtered, w)
			}
		}
		if len(filtered) > 0 {
			pool = filtered
		}
	case LengthLong:
		var filtered []string
		for _, w := range c.words {
			if len(w) >= 6 {
				filtered = append(filtered, w)
			}
		}
		if len(filtered) > 0 {
			pool = filtered
		}
	}

	// Fall back to all words if pool is empty
	if len(pool) == 0 {
		pool = c.words
	}

	// Return empty string if still empty
	if len(pool) == 0 {
		return ""
	}

	// Deterministic selection: ensure non-negative index
	index := ((seed % len(pool)) + len(pool)) % len(pool)
	return pool[index]
}

// Sentence returns a practice sentence, chosen by seed.
// The selection is deterministic: same seed yields the same sentence.
func (c *staticCorpus) Sentence(seed int) string {
	if len(c.sentences) == 0 {
		return ""
	}
	// Deterministic selection: ensure non-negative index
	index := ((seed % len(c.sentences)) + len(c.sentences)) % len(c.sentences)
	return c.sentences[index]
}

// newStaticCorpus returns a new Corpus backed by the hardcoded word and sentence bank.
func newStaticCorpus() Corpus {
	// Raw word string, space-separated, copied verbatim from prototype
	rawWords := "the be to of and a in that have it for not on with he as you do at this but his by from they we say her she or an will my one all would there their what so up out if about who get which go me when make can like time no just him know take people into year your good some could them see other than then now look only come over think back after use two how work first well way even want because these give day most water long very find here thing great little world still hand high right small large next early young important few public bad same able story mind party music course between country problem hour game line end member law car city community name president team minute idea body information nothing money result change morning reason research girl guy moment air teacher force education plan door number wall paper number able wide door left home moment food heart night true friend fact street table light front rather learn need feel become leave nature better school white house sound wind field paint drink dance simple happy quiet build sleep table plant water river green brown black round sharp smart clean quick brave clear fresh sweet bread cream stone metal glass color voice space light music dream heart peace world value trust honor pride craft skill trade grade grace price prize drive alive olive plane plate stage stone smoke flame frame shade share stare spare spice slice pride bride guide slide glide chase phase phrase praise"

	// Split on whitespace, filter by ^[a-z]+$, deduplicate preserving first-seen order
	tokens := strings.Fields(rawWords)
	seen := make(map[string]bool)
	var words []string
	for _, token := range tokens {
		if isLowerAlpha(token) && !seen[token] {
			seen[token] = true
			words = append(words, token)
		}
	}

	// Hardcoded sentences, copied verbatim from prototype
	sentences := []string{
		"the quick brown fox jumps over the lazy dog",
		"she sells sea shells by the shore each day",
		"we should leave before the rain starts again",
		"a little practice every morning goes a long way",
		"the old house stood quiet at the end of the lane",
		"they found the missing key under the front door mat",
		"good work takes time patience and a clear mind",
		"the river runs slow past the green fields of home",
		"he wrote a short note and left it on the table",
		"learning a new layout feels strange at first",
		"keep your hands relaxed and let the words flow",
		"the best way to improve is to type a little each day",
		"bright stars filled the sky above the sleeping town",
		"she paints the walls a warm shade of soft yellow",
		"we walked along the beach and watched the waves roll in",
		"a strong cup of coffee helps me start the day right",
		"the plan was simple but the work proved much harder",
		"trust takes years to build and moments to break",
		"the small dog ran across the yard chasing a ball",
		"clear skies and a cool breeze made for a fine day",
	}

	return &staticCorpus{
		words:     words,
		sentences: sentences,
	}
}
