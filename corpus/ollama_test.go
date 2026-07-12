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

	if o.Temperature != defaultTemperature || o.RepeatPenalty != defaultRepeatPenalty {
		t.Errorf("sampling defaults = %v/%v, want %v/%v",
			o.Temperature, o.RepeatPenalty, defaultTemperature, defaultRepeatPenalty)
	}
}

// TestOptionsSampling pins the three-way meaning of a sampling field: unset
// takes the corpus default, an explicit value is sent as-is, and a negative
// value omits the option so the model's own declared parameters stand.
func TestOptionsSampling(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want map[string]any
	}{
		{
			name: "zero values take the corpus defaults",
			opts: Options{},
			want: map[string]any{"temperature": defaultTemperature, "repeat_penalty": defaultRepeatPenalty},
		},
		{
			name: "explicit values are sent as-is",
			opts: Options{Temperature: 0.2, RepeatPenalty: 1.4},
			want: map[string]any{"temperature": 0.2, "repeat_penalty": 1.4},
		},
		{
			name: "negative temperature defers to the model",
			opts: Options{Temperature: -1},
			want: map[string]any{"repeat_penalty": defaultRepeatPenalty},
		},
		{
			name: "both negative sends no options at all",
			opts: Options{Temperature: -1, RepeatPenalty: -1},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.opts.withDefaults().sampling()
			if len(got) != len(tt.want) {
				t.Fatalf("sampling() = %v, want %v", got, tt.want)
			}
			for k, want := range tt.want {
				if got[k] != want {
					t.Errorf("sampling()[%q] = %v, want %v", k, got[k], want)
				}
			}
		})
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
