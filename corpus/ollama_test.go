package corpus

import (
	"reflect"
	"testing"
)

// Note: these tests only exercise the pure parsing helpers. No live Ollama
// server is contacted anywhere in this package's tests.

const sampleLLMWordsResponse = `1. apple
banana
 cherry
Date!
2. elderberry
apple

fig-newton
GRAPE
123
kiwi.`

func TestParseWordLines(t *testing.T) {
	got := parseWordLines(sampleLLMWordsResponse)
	want := []string{"apple", "banana", "cherry", "date", "elderberry", "fig", "grape", "kiwi"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseWordLines = %#v, want %#v", got, want)
	}
}

const sampleLLMSentencesResponse = `1. the cat sat on the mat today
The Cat Sat On The Mat Today!

hi
2. we walked to the store this morning
short line here
a bright sun rose over the quiet hills`

func TestParseSentenceLines(t *testing.T) {
	got := parseSentenceLines(sampleLLMSentencesResponse)
	want := []string{
		"the cat sat on the mat today",
		"we walked to the store this morning",
		"a bright sun rose over the quiet hills",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseSentenceLines = %#v, want %#v", got, want)
	}
	// "short line here" only has 3 words and must be filtered out by the
	// >= 4 words rule.
	for _, s := range got {
		if s == "short line here" {
			t.Errorf("expected the 3-word line to be filtered out, got %v", got)
		}
	}
}

func TestOptionsWithDefaults(t *testing.T) {
	o := Options{}.withDefaults()
	if o.Model != defaultModel {
		t.Errorf("Model = %q, want %q", o.Model, defaultModel)
	}
	if o.NumWords != defaultNumWords {
		t.Errorf("NumWords = %d, want %d", o.NumWords, defaultNumWords)
	}
	if o.NumSentences != defaultNumSentences {
		t.Errorf("NumSentences = %d, want %d", o.NumSentences, defaultNumSentences)
	}

	custom := Options{Model: "mistral", NumWords: 50, NumSentences: 5}.withDefaults()
	if custom.Model != "mistral" || custom.NumWords != 50 || custom.NumSentences != 5 {
		t.Errorf("withDefaults changed explicit values: %#v", custom)
	}
}

func TestPromptsMentionCounts(t *testing.T) {
	if got := wordsPrompt(42); !contains(got, "42") {
		t.Errorf("wordsPrompt(42) does not mention count: %q", got)
	}
	if got := sentencesPrompt(7); !contains(got, "7") {
		t.Errorf("sentencesPrompt(7) does not mention count: %q", got)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
