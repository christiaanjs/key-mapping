package core

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

// ---- fakes: deterministic, independent of the static bank ----

// fakeMapping treats every character as supported and mappable except those
// explicitly listed in unsupported. Hint/Diagnose are intentionally trivial:
// they let the drill/App tests focus on App behaviour, not mapping semantics
// (mapping_static_test.go already covers the real mapping).
type fakeMapping struct {
	unsupported map[rune]bool
	diagnoseMsg string
}

func newFakeMapping() *fakeMapping {
	return &fakeMapping{
		unsupported: map[rune]bool{},
		diagnoseMsg: "diagnose-sentinel",
	}
}

func (m *fakeMapping) Hint(output rune) KeyHint {
	if m.unsupported[output] {
		return KeyHint{Output: string(output), Mapped: false}
	}
	if output == ' ' {
		return KeyHint{Output: " ", IsSpace: true, Mapped: true}
	}
	return KeyHint{Output: string(output), Key: string(output), Mapped: true}
}

func (m *fakeMapping) Supported(output rune) bool {
	return !m.unsupported[output]
}

func (m *fakeMapping) Diagnose(want, got rune) string {
	return m.diagnoseMsg
}

func (m *fakeMapping) Reference() []RefRow {
	return []RefRow{{Key: "x", Output: "y"}}
}

// fakeCorpus always returns the same fixed word/sentence regardless of the
// length filter or seed, so drills are deterministic in these tests.
type fakeCorpus struct {
	word     string
	sentence string
}

func (c *fakeCorpus) Word(length Length, seed int) string { return c.word }
func (c *fakeCorpus) Sentence(seed int) string            { return c.sentence }

func newFakeApp() *App {
	return New(newFakeMapping(), &fakeCorpus{word: "cat", sentence: "the cat sat"})
}

// ---- mirror drill: typing ----

func TestApp_MirrorTypeCorrect_AdvancesAndCompletesWord(t *testing.T) {
	app := newFakeApp() // word: "cat"
	app.Dispatch(Event{Type: EvSwitchMode, Mode: ModeMirror})

	st := app.Dispatch(Event{Type: EvType, Rune: "c", AtMillis: 1000})
	if st.Drill.Index != 1 {
		t.Fatalf("after 'c': Index = %d, want 1", st.Drill.Index)
	}
	if st.Drill.Stats.Hits != 1 {
		t.Fatalf("after 'c': Hits = %d, want 1", st.Drill.Stats.Hits)
	}

	st = app.Dispatch(Event{Type: EvType, Rune: "a", AtMillis: 1100})
	if st.Drill.Index != 2 {
		t.Fatalf("after 'a': Index = %d, want 2", st.Drill.Index)
	}
	if st.Drill.Stats.Hits != 2 {
		t.Fatalf("after 'a': Hits = %d, want 2", st.Drill.Stats.Hits)
	}

	// Final character: completes the word, and the App immediately starts
	// the next item (same fixed word, from the fake corpus), so the
	// snapshot after completion shows the *next* word already reset to
	// Index 0, with the completion feedback preserved.
	st = app.Dispatch(Event{Type: EvType, Rune: "t", AtMillis: 1200})
	if st.Drill.Stats.Hits != 3 {
		t.Fatalf("after 't': Hits = %d, want 3", st.Drill.Stats.Hits)
	}
	if st.Drill.Feedback != "Word complete!" {
		t.Fatalf("after 't': Feedback = %q, want %q", st.Drill.Feedback, "Word complete!")
	}
	if st.Drill.Index != 0 {
		t.Fatalf("after completion: Index = %d, want 0 (next word started)", st.Drill.Index)
	}
	if st.Drill.Text != "cat" {
		t.Fatalf("after completion: Text = %q, want %q (fixed corpus)", st.Drill.Text, "cat")
	}
}

func TestApp_MirrorTypeWrong_RecordsMissAndDiagnoses(t *testing.T) {
	app := newFakeApp() // word: "cat", target[0] == 'c'
	app.Dispatch(Event{Type: EvSwitchMode, Mode: ModeMirror})

	st := app.Dispatch(Event{Type: EvType, Rune: "x", AtMillis: 1000})

	if st.Drill.Stats.Misses != 1 {
		t.Fatalf("Misses = %d, want 1", st.Drill.Stats.Misses)
	}
	if st.Drill.Stats.Hits != 0 {
		t.Fatalf("Hits = %d, want 0", st.Drill.Stats.Hits)
	}
	if st.Drill.Feedback != "diagnose-sentinel" {
		t.Fatalf("Feedback = %q, want %q", st.Drill.Feedback, "diagnose-sentinel")
	}
	if st.Drill.Index != 0 {
		t.Fatalf("Index = %d, want 0 (miss must not advance)", st.Drill.Index)
	}
}

func TestApp_MirrorUnsupportedChar_AutoSkipped(t *testing.T) {
	m := newFakeMapping()
	m.unsupported['b'] = true
	app := New(m, &fakeCorpus{word: "bat", sentence: "x"})

	st := app.Dispatch(Event{Type: EvSwitchMode, Mode: ModeMirror})
	if st.Drill.Index != 1 {
		t.Fatalf("Index after init = %d, want 1 ('b' should be auto-skipped)", st.Drill.Index)
	}

	// Typing the (now current) 'a' should hit normally.
	st = app.Dispatch(Event{Type: EvType, Rune: "a", AtMillis: 1000})
	if st.Drill.Index != 2 {
		t.Fatalf("Index after 'a' = %d, want 2", st.Drill.Index)
	}
	if st.Drill.Stats.Hits != 1 {
		t.Fatalf("Hits after 'a' = %d, want 1", st.Drill.Stats.Hits)
	}

	st = app.Dispatch(Event{Type: EvType, Rune: "t", AtMillis: 1100})
	if st.Drill.Feedback != "Word complete!" {
		t.Fatalf("Feedback after 't' = %q, want %q", st.Drill.Feedback, "Word complete!")
	}
	if st.Drill.Stats.Hits != 2 {
		t.Fatalf("Hits after 't' = %d, want 2", st.Drill.Stats.Hits)
	}
}

// ---- accuracy ----

func TestApp_Accuracy(t *testing.T) {
	t.Run("zero attempts is 100", func(t *testing.T) {
		app := newFakeApp()
		st := app.Dispatch(Event{Type: EvSwitchMode, Mode: ModeMirror})
		if st.Drill.Stats.Accuracy != 100 {
			t.Fatalf("Accuracy = %d, want 100", st.Drill.Stats.Accuracy)
		}
	})

	t.Run("3 hits 1 miss is 75", func(t *testing.T) {
		// word "aaaa": every position wants 'a', so we can rack up misses
		// without ever advancing past the same target character.
		app := New(newFakeMapping(), &fakeCorpus{word: "aaaa", sentence: "x"})
		app.Dispatch(Event{Type: EvSwitchMode, Mode: ModeMirror})

		app.Dispatch(Event{Type: EvType, Rune: "x", AtMillis: 1000})       // miss
		app.Dispatch(Event{Type: EvType, Rune: "a", AtMillis: 1001})       // hit
		app.Dispatch(Event{Type: EvType, Rune: "a", AtMillis: 1002})       // hit
		st := app.Dispatch(Event{Type: EvType, Rune: "a", AtMillis: 1003}) // hit

		if st.Drill.Stats.Hits != 3 || st.Drill.Stats.Misses != 1 {
			t.Fatalf("Hits/Misses = %d/%d, want 3/1", st.Drill.Stats.Hits, st.Drill.Stats.Misses)
		}
		if st.Drill.Stats.Accuracy != 75 {
			t.Fatalf("Accuracy = %d, want 75", st.Drill.Stats.Accuracy)
		}
	})
}

// ---- WPM ----

func TestApp_WPM(t *testing.T) {
	// word "abcdef" (6 chars): type the first 4 correctly so the word never
	// completes (which would reset wpm to 0), then check the reported WPM
	// against an independently computed (chars/5)/minutes formula.
	app := New(newFakeMapping(), &fakeCorpus{word: "abcdef", sentence: "x"})
	app.Dispatch(Event{Type: EvSwitchMode, Mode: ModeMirror})

	start := int64(1000)
	times := []int64{start, start + 6000, start + 12000, start + 18000}
	chars := []string{"a", "b", "c", "d"}

	var st State
	for i, ch := range chars {
		st = app.Dispatch(Event{Type: EvType, Rune: ch, AtMillis: times[i]})
	}

	elapsedMinutes := float64(times[3]-start) / 60000.0
	words := float64(4) / 5.0
	wantWPM := int(math.Round(words / elapsedMinutes))

	if wantWPM <= 0 {
		t.Fatalf("test setup error: expected wantWPM > 0, got %d", wantWPM)
	}
	if st.Drill.Stats.WPM != wantWPM {
		t.Fatalf("WPM = %d, want %d (elapsed=%.4fmin, words=%.2f)", st.Drill.Stats.WPM, wantWPM, elapsedMinutes, words)
	}
}

// ---- nav drill ----

func TestApp_NavSwitchMode_PopulatesSequence(t *testing.T) {
	app := newFakeApp()
	st := app.Dispatch(Event{Type: EvSwitchMode, Mode: ModeNav})

	if st.Nav == nil {
		t.Fatalf("Nav = nil, want populated NavState")
	}
	if len(st.Nav.Sequence) != 6 {
		t.Fatalf("len(Sequence) = %d, want 6", len(st.Nav.Sequence))
	}
	if st.Nav.Index != 0 || st.Nav.Hits != 0 || st.Nav.Misses != 0 {
		t.Fatalf("fresh nav state = %+v, want Index/Hits/Misses all 0", st.Nav)
	}
}

func TestApp_NavCorrectArrow_AdvancesHits(t *testing.T) {
	app := newFakeApp()
	st := app.Dispatch(Event{Type: EvSwitchMode, Mode: ModeNav})
	want := st.Nav.Sequence[0]

	st = app.Dispatch(Event{Type: EvArrow, Arrow: want})
	if st.Nav.Hits != 1 {
		t.Fatalf("Hits = %d, want 1", st.Nav.Hits)
	}
	if st.Nav.Index != 1 {
		t.Fatalf("Index = %d, want 1", st.Nav.Index)
	}
	if st.Nav.Feedback != "" {
		t.Fatalf("Feedback = %q, want empty", st.Nav.Feedback)
	}
}

func TestApp_NavWrongArrow_RecordsMissAndFeedback(t *testing.T) {
	app := newFakeApp()
	st := app.Dispatch(Event{Type: EvSwitchMode, Mode: ModeNav})
	correct := st.Nav.Sequence[0]

	arrows := []string{ArrowLeft, ArrowDown, ArrowUp, ArrowRight}
	var wrong string
	for _, a := range arrows {
		if a != correct {
			wrong = a
			break
		}
	}

	st = app.Dispatch(Event{Type: EvArrow, Arrow: wrong})
	if st.Nav.Misses != 1 {
		t.Fatalf("Misses = %d, want 1", st.Nav.Misses)
	}
	if st.Nav.Index != 0 {
		t.Fatalf("Index = %d, want 0 (miss must not advance)", st.Nav.Index)
	}
	if !strings.Contains(st.Nav.Feedback, "wanted") {
		t.Fatalf("Feedback = %q, want it to mention what was wanted", st.Nav.Feedback)
	}
}

func TestApp_NavCompleteSequence_GeneratesFreshSequence(t *testing.T) {
	app := newFakeApp()
	st := app.Dispatch(Event{Type: EvSwitchMode, Mode: ModeNav})
	seq := append([]string(nil), st.Nav.Sequence...)

	for _, arrow := range seq {
		st = app.Dispatch(Event{Type: EvArrow, Arrow: arrow})
	}

	if st.Nav.Index != 0 {
		t.Fatalf("Index after wrap = %d, want 0 (fresh sequence)", st.Nav.Index)
	}
	if st.Nav.Hits != 0 || st.Nav.Misses != 0 {
		t.Fatalf("Hits/Misses after wrap = %d/%d, want 0/0 (fresh sequence resets counters)", st.Nav.Hits, st.Nav.Misses)
	}
	if len(st.Nav.Sequence) != 6 {
		t.Fatalf("len(Sequence) after wrap = %d, want 6", len(st.Nav.Sequence))
	}
}

func TestApp_NavSequenceDeterministic(t *testing.T) {
	app1 := newFakeApp()
	app2 := newFakeApp()

	st1 := app1.Dispatch(Event{Type: EvSwitchMode, Mode: ModeNav})
	st2 := app2.Dispatch(Event{Type: EvSwitchMode, Mode: ModeNav})

	if !reflect.DeepEqual(st1.Nav.Sequence, st2.Nav.Sequence) {
		t.Fatalf("sequences differ: %v vs %v, want identical (pick() must be deterministic)", st1.Nav.Sequence, st2.Nav.Sequence)
	}
}

// ---- mode routing ----

func TestApp_ModeRouting(t *testing.T) {
	tests := []struct {
		name       string
		mode       Mode
		wantDrill  bool
		wantNav    bool
		wantRefLen int // -1 means "want nil"
	}{
		{"mirror populates drill only", ModeMirror, true, false, -1},
		{"nav populates nav only", ModeNav, false, true, -1},
		{"reference populates reference only", ModeReference, false, false, 1},
		{"scratch populates nothing", ModeScratch, false, false, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newFakeApp()
			st := app.Dispatch(Event{Type: EvSwitchMode, Mode: tt.mode})

			if (st.Drill != nil) != tt.wantDrill {
				t.Errorf("Drill != nil = %v, want %v", st.Drill != nil, tt.wantDrill)
			}
			if (st.Nav != nil) != tt.wantNav {
				t.Errorf("Nav != nil = %v, want %v", st.Nav != nil, tt.wantNav)
			}
			if tt.wantRefLen == -1 {
				if st.Reference != nil {
					t.Errorf("Reference = %v, want nil", st.Reference)
				}
			} else if len(st.Reference) != tt.wantRefLen {
				t.Errorf("len(Reference) = %d, want %d", len(st.Reference), tt.wantRefLen)
			}
		})
	}
}
