package corpus

import (
	"testing"

	"github.com/christiaanswanepoel/key-mapping/core"
)

func newTestSource() *Source {
	return &Source{
		words:     []string{"cat", "dog", "bird", "elephant", "giraffe", "ox"},
		sentences: []string{"the cat sat on the mat", "the dog ran in the park"},
	}
}

func TestSourceWordLengthFilter(t *testing.T) {
	s := newTestSource()

	// LengthShort: only words with len <= 4 -> "cat","dog","bird","ox"
	for seed := 0; seed < 8; seed++ {
		w := s.Word(core.LengthShort, seed)
		if len(w) > 4 {
			t.Errorf("LengthShort seed=%d returned %q (len %d > 4)", seed, w, len(w))
		}
	}

	// LengthLong: only words with len >= 6 -> "elephant","giraffe"
	for seed := 0; seed < 8; seed++ {
		w := s.Word(core.LengthLong, seed)
		if len(w) < 6 {
			t.Errorf("LengthLong seed=%d returned %q (len %d < 6)", seed, w, len(w))
		}
	}

	// LengthAny: any word from the full pool.
	w := s.Word(core.LengthAny, 0)
	if w == "" {
		t.Errorf("LengthAny returned empty string")
	}
}

func TestSourceWordDeterministic(t *testing.T) {
	s := newTestSource()
	for seed := 0; seed < 20; seed++ {
		a := s.Word(core.LengthAny, seed)
		b := s.Word(core.LengthAny, seed)
		if a != b {
			t.Errorf("seed=%d: Word not deterministic: %q vs %q", seed, a, b)
		}
	}
}

func TestSourceWordNegativeSeed(t *testing.T) {
	s := newTestSource()
	w := s.Word(core.LengthAny, -3)
	if w == "" {
		t.Errorf("negative seed produced empty word")
	}
}

func TestSourceWordEmptyFallback(t *testing.T) {
	// No words at all -> empty string regardless of filter.
	s := &Source{words: nil, sentences: nil}
	if got := s.Word(core.LengthShort, 0); got != "" {
		t.Errorf("expected empty string for empty word pool, got %q", got)
	}

	// Words exist but none satisfy the length filter -> falls back to all words.
	s2 := &Source{words: []string{"elephant", "giraffe"}}
	if got := s2.Word(core.LengthShort, 0); got == "" {
		t.Errorf("expected fallback to full pool, got empty string")
	}
}

func TestSourceSentence(t *testing.T) {
	s := newTestSource()

	for seed := 0; seed < 10; seed++ {
		a := s.Sentence(seed)
		b := s.Sentence(seed)
		if a != b {
			t.Errorf("seed=%d: Sentence not deterministic: %q vs %q", seed, a, b)
		}
		if a == "" {
			t.Errorf("seed=%d: Sentence returned empty string with non-empty pool", seed)
		}
	}

	empty := &Source{}
	if got := empty.Sentence(0); got != "" {
		t.Errorf("expected empty string for empty sentence pool, got %q", got)
	}
}

func TestSourceImplementsCorpus(t *testing.T) {
	var _ core.Corpus = &Source{}
}
