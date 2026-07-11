package corpus

import (
	"strings"
	"testing"
)

// Note: these tests only exercise pure helpers (prompt building, defaults,
// line cleaning). No live Ollama server is contacted anywhere in this
// package's tests; see ollama_live_test.go for the opt-in live test.

func TestOptionsWithDefaults(t *testing.T) {
	o := Options{}.withDefaults()
	if o.Model != defaultModel {
		t.Errorf("Model = %q, want %q", o.Model, defaultModel)
	}

	custom := Options{Model: "mistral"}.withDefaults()
	if custom.Model != "mistral" {
		t.Errorf("withDefaults changed explicit Model: %#v", custom)
	}
}

func TestPromptForMentionsCountAndVariesByRound(t *testing.T) {
	wordPrompt := promptFor(KindWord, 42, 0)
	if !strings.Contains(wordPrompt, "42") {
		t.Errorf("word prompt does not mention count: %q", wordPrompt)
	}
	sentPrompt := promptFor(KindSentence, 7, 0)
	if !strings.Contains(sentPrompt, "7") {
		t.Errorf("sentence prompt does not mention count: %q", sentPrompt)
	}

	// Different rounds should (eventually) pick different themes so repeated
	// calls don't regenerate the same list.
	seenThemes := make(map[string]bool)
	for round := int64(0); round < int64(len(themes)); round++ {
		p := promptFor(KindWord, 10, round)
		seenThemes[p] = true
	}
	if len(seenThemes) < 2 {
		t.Errorf("expected prompts to vary across rounds, got %d distinct prompts over %d rounds", len(seenThemes), len(themes))
	}
}

func TestEmitLineWord(t *testing.T) {
	var got []string
	emit := func(s string) { got = append(got, s) }

	emitLine(KindWord, "1. Apple!", emit)
	emitLine(KindWord, "", emit)
	emitLine(KindWord, "123", emit)
	emitLine(KindWord, "banana extra stuff", emit)

	want := []string{"apple", "banana"}
	if len(got) != len(want) {
		t.Fatalf("emitLine(KindWord) = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("emitLine(KindWord)[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEmitLineSentence(t *testing.T) {
	var got []string
	emit := func(s string) { got = append(got, s) }

	emitLine(KindSentence, "The Cat Sat On The Mat!", emit)
	emitLine(KindSentence, "hi there", emit) // too few words
	emitLine(KindSentence, "", emit)

	want := []string{"the cat sat on the mat"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("emitLine(KindSentence) = %#v, want %#v", got, want)
	}
}
