package core

import (
	"fmt"
	"math"
)

// App is the pure trainer state machine. It holds no platform-specific state
// (no clock, no I/O) so it compiles for both the native TUI and the
// GOOS=js/GOARCH=wasm web target. All mutation flows through Dispatch, and
// Snapshot renders the current State for whichever Mode is active.
type App struct {
	mapping Mapping
	corpus  Corpus

	mode    Mode
	content Content
	length  Length

	// Mirror-mode drill (words/sentences). hits/misses persist across items
	// within a mode, matching the prototype's keepStats behaviour.
	text        string
	idx         int
	hits        int
	misses      int
	startMillis int64
	wpm         int
	feedback    string

	// Nav-mode arrow-sequence drill. hits/misses/feedback reset each time a
	// new sequence is generated.
	navSeq      []string
	navIdx      int
	navHits     int
	navMisses   int
	navFeedback string

	// pick is incremented every time a new drill item or nav sequence is
	// generated; it is passed to the Corpus and to the nav sequence generator
	// as a deterministic seed.
	pick int
}

// New constructs an App wired to the given Mapping and Corpus, starting in
// ModeMirror with ContentWords/LengthAny.
func New(m Mapping, c Corpus) *App {
	return &App{
		mapping: m,
		corpus:  c,
		mode:    ModeMirror,
		content: ContentWords,
		length:  LengthAny,
	}
}

// NewDefault constructs an App using the hardcoded static mapping and corpus.
func NewDefault() *App { return New(newStaticMapping(), newStaticCorpus()) }

// Dispatch applies ev to the state machine and returns the resulting State.
func (a *App) Dispatch(ev Event) State {
	switch ev.Type {
	case EvSwitchMode:
		a.mode = ev.Mode
		switch a.mode {
		case ModeMirror:
			a.ensureMirrorInit()
		case ModeNav:
			a.ensureNavInit()
		}

	case EvSetContent:
		a.content = ev.Content
		a.startMirrorItem()
		a.feedback = ""

	case EvSetLength:
		a.length = ev.Length
		a.startMirrorItem()
		a.feedback = ""

	case EvType:
		if a.mode == ModeMirror {
			a.handleType(ev)
		}
		// ModeScratch (and any other mode) ignores EvType: the frontend
		// echoes the live keymap itself, with no server-side state.

	case EvArrow:
		if a.mode == ModeNav {
			a.handleArrow(ev)
		}

	case EvSkip:
		a.startMirrorItem()
		a.feedback = ""
	}

	return a.Snapshot()
}

// Snapshot renders the current State for the active mode without mutating
// anything other than lazily initializing drill state that has never been
// touched (so calling Snapshot before any Dispatch still returns a usable
// State).
func (a *App) Snapshot() State {
	switch a.mode {
	case ModeMirror:
		a.ensureMirrorInit()
	case ModeNav:
		a.ensureNavInit()
	}

	st := State{
		Mode:    a.mode,
		Content: a.content,
		Length:  a.length,
	}

	switch a.mode {
	case ModeMirror:
		st.Drill = a.buildDrillState()
	case ModeNav:
		st.Nav = a.buildNavState()
	case ModeReference:
		st.Reference = a.mapping.Reference()
	case ModeScratch:
		// no server-side state
	}

	return st
}

// ---- mirror drill ----

func (a *App) ensureMirrorInit() {
	if a.text == "" {
		a.startMirrorItem()
	}
}

// startMirrorItem picks a new drill item from the corpus, resetting idx and
// startMillis/wpm but leaving hits/misses/feedback untouched (callers decide
// whether to clear feedback).
func (a *App) startMirrorItem() {
	a.pick++
	if a.content == ContentSentences {
		a.text = a.corpus.Sentence(a.pick)
	} else {
		a.text = a.corpus.Word(a.length, a.pick)
	}
	a.idx = 0
	a.startMillis = 0
	a.wpm = 0
	a.skipUnsupported()
}

// currentTarget returns the rune at a.idx and whether one exists.
func (a *App) currentTarget() (rune, bool) {
	rs := []rune(a.text)
	if a.idx < 0 || a.idx >= len(rs) {
		return 0, false
	}
	return rs[a.idx], true
}

// skipUnsupported advances idx past any target characters the mapping cannot
// produce.
func (a *App) skipUnsupported() {
	rs := []rune(a.text)
	for a.idx < len(rs) && !a.mapping.Supported(rs[a.idx]) {
		a.idx++
	}
}

func (a *App) handleType(ev Event) {
	rs := []rune(ev.Rune)
	if len(rs) == 0 {
		return
	}
	R := rs[0]

	a.skipUnsupported()
	target, ok := a.currentTarget()
	if !ok {
		return
	}

	if a.startMillis == 0 {
		a.startMillis = ev.AtMillis
	}

	if R == target {
		a.idx++
		a.hits++
		a.skipUnsupported()
		a.wpm = calcWPM(a.idx, a.startMillis, ev.AtMillis)

		if _, has := a.currentTarget(); !has {
			if a.content == ContentSentences {
				a.feedback = "Sentence done!"
			} else {
				a.feedback = "Word complete!"
			}
			// Immediately start the next item, keeping hits/misses and the
			// completion feedback just set (startMirrorItem never touches
			// feedback).
			a.startMirrorItem()
		}
	} else {
		a.misses++
		a.feedback = a.mapping.Diagnose(target, R)
		a.wpm = calcWPM(a.idx, a.startMillis, ev.AtMillis)
	}
}

func (a *App) buildDrillState() *DrillState {
	rs := []rune(a.text)
	chars := make([]DrillChar, len(rs))
	for i, r := range rs {
		var status CharStatus
		switch {
		case i < a.idx:
			status = CharDone
		case i == a.idx:
			status = CharCurrent
		default:
			status = CharPending
		}
		chars[i] = DrillChar{Char: string(r), Status: status}
	}

	target, ok := a.currentTarget()
	var hint KeyHint
	if ok {
		hint = a.mapping.Hint(target)
	}

	return &DrillState{
		Text:     a.text,
		Chars:    chars,
		Index:    a.idx,
		Hint:     hint,
		Feedback: a.feedback,
		Complete: !ok,
		Stats: Stats{
			Hits:     a.hits,
			Misses:   a.misses,
			Accuracy: accuracy(a.hits, a.misses),
			WPM:      a.wpm,
		},
	}
}

// ---- nav drill ----

func (a *App) ensureNavInit() {
	if a.navSeq == nil {
		a.startNavSequence()
	}
}

func (a *App) startNavSequence() {
	a.pick++
	dirs := [4]string{ArrowLeft, ArrowDown, ArrowUp, ArrowRight}
	seq := make([]string, 6)
	for i := range seq {
		seq[i] = dirs[pick(len(dirs), a.pick+i)]
	}
	a.navSeq = seq
	a.navIdx = 0
	a.navHits = 0
	a.navMisses = 0
	a.navFeedback = ""
}

func (a *App) handleArrow(ev Event) {
	if len(a.navSeq) == 0 {
		return
	}
	want := a.navSeq[a.navIdx]
	if ev.Arrow == want {
		a.navIdx++
		a.navHits++
		a.navFeedback = ""
		if a.navIdx >= len(a.navSeq) {
			a.startNavSequence()
		}
	} else {
		a.navMisses++
		a.navFeedback = fmt.Sprintf("Got %s, wanted %s. (a=left s=down d=up f=right)", ev.Arrow, want)
	}
}

func (a *App) buildNavState() *NavState {
	seq := make([]string, len(a.navSeq))
	copy(seq, a.navSeq)
	return &NavState{
		Sequence: seq,
		Index:    a.navIdx,
		Hits:     a.navHits,
		Misses:   a.navMisses,
		Feedback: a.navFeedback,
	}
}

// ---- shared helpers ----

// accuracy returns the hit percentage, rounded, or 100 when nothing has been
// typed yet.
func accuracy(hits, misses int) int {
	total := hits + misses
	if total == 0 {
		return 100
	}
	return int(math.Round(float64(hits) / float64(total) * 100))
}

// calcWPM computes words-per-minute from the number of characters typed so
// far (idx), the millis timestamp the item was started, and the millis
// timestamp of the current event. It returns 0 until enough time has passed
// to measure.
func calcWPM(idx int, startMillis, atMillis int64) int {
	if startMillis <= 0 || atMillis <= startMillis {
		return 0
	}
	elapsedMinutes := float64(atMillis-startMillis) / 60000.0
	if elapsedMinutes <= 0 {
		return 0
	}
	words := float64(idx) / 5.0
	return int(math.Round(words / elapsedMinutes))
}

// pick returns a deterministic, non-negative index in [0, n) derived from
// seed via a small integer hash (splitmix-style mix). It guards n<=0 by
// returning 0. Used so the same seed always yields the same pick, keeping
// drills reproducible/testable.
func pick(n, seed int) int {
	if n <= 0 {
		return 0
	}
	x := uint32(int32(seed))
	x += 0x9E3779B9
	x ^= x >> 15
	x *= 2246822519
	x ^= x >> 13
	x *= 3266489917
	x ^= x >> 16
	return int(x % uint32(n))
}
