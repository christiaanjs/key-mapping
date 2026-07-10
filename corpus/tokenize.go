package corpus

import "strings"

// isLowerAlpha reports whether s is non-empty and all ASCII a-z.
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

// Words extracts lowercase [a-z]+ tokens from text: the text is lowercased,
// split on any run of non-letter characters, and the resulting tokens are
// deduplicated while preserving first-seen order.
func Words(text string) []string {
	lower := strings.ToLower(text)

	seen := make(map[string]bool)
	var words []string
	var b strings.Builder

	flush := func() {
		if b.Len() == 0 {
			return
		}
		w := b.String()
		b.Reset()
		if isLowerAlpha(w) && !seen[w] {
			seen[w] = true
			words = append(words, w)
		}
	}

	for _, r := range lower {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()

	return words
}

// Sentences extracts practice sentences from text, one per input line: each
// line is lowercased, stripped down to a-z and spaces only, has its
// whitespace collapsed to single spaces, and is trimmed. Lines with fewer
// than 4 resulting words are dropped. Results are deduplicated preserving
// first-seen order.
func Sentences(text string) []string {
	lines := strings.Split(text, "\n")

	seen := make(map[string]bool)
	var sentences []string

	for _, line := range lines {
		lower := strings.ToLower(line)

		var b strings.Builder
		for _, r := range lower {
			switch {
			case r >= 'a' && r <= 'z':
				b.WriteRune(r)
			case r == ' ' || r == '\t':
				b.WriteRune(' ')
			default:
				// drop punctuation/digits/other runes entirely
			}
		}

		// Collapse whitespace to single spaces and trim.
		fields := strings.Fields(b.String())
		if len(fields) < 4 {
			continue
		}
		sentence := strings.Join(fields, " ")

		if !seen[sentence] {
			seen[sentence] = true
			sentences = append(sentences, sentence)
		}
	}

	return sentences
}
