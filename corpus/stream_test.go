package corpus

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/christiaanswanepoel/key-mapping/core"
)

// fixedCorpus is a trivial core.Corpus used as Stream's fallback in tests.
type fixedCorpus struct {
	word     string
	sentence string
}

func (f *fixedCorpus) Word(core.Length, int) string { return f.word }
func (f *fixedCorpus) Sentence(int) string          { return f.sentence }

// fakeProducer is a controllable Producer: tests set fn to whatever behavior
// a case needs (emit items, block on a gate, always error, ...) and can read
// back how many times Produce was called.
type fakeProducer struct {
	mu    sync.Mutex
	fn    func(ctx context.Context, kind Kind, n int, emit func(string)) error
	calls int
}

func (f *fakeProducer) Produce(ctx context.Context, kind Kind, n int, emit func(string)) error {
	f.mu.Lock()
	f.calls++
	fn := f.fn
	f.mu.Unlock()

	if fn == nil {
		return nil
	}
	return fn(ctx, kind, n, emit)
}

func (f *fakeProducer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// waitFor polls cond until it's true or the timeout elapses, failing the
// test instead of sleeping blindly for a fixed duration.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !cond() {
		t.Fatalf("condition not met within %s", timeout)
	}
}

// emitN emits n unique, deterministic items for kind, numbered starting at
// start, so repeated calls across a test don't collide with the dedup set.
func emitN(kind Kind, n int, start *int64, emit func(string)) {
	for i := 0; i < n; i++ {
		id := atomic.AddInt64(start, 1)
		if kind == KindWord {
			emit(fmt.Sprintf("w%d", id))
		} else {
			emit(fmt.Sprintf("sentence number %d has enough words", id))
		}
	}
}

func TestStreamFallsBackColdThenWarmsUp(t *testing.T) {
	fallback := &fixedCorpus{word: "fallback", sentence: "fallback sentence with words"}

	gate := make(chan struct{})
	var counter int64
	fp := &fakeProducer{fn: func(ctx context.Context, kind Kind, n int, emit func(string)) error {
		<-gate
		emitN(kind, n, &counter, emit)
		return nil
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewStream(ctx, fp, "fake", fallback)
	defer s.Close()

	// Cold: nothing generated yet, must serve fallback and report warming.
	if got := s.Word(core.LengthAny, 0); got != "fallback" {
		t.Errorf("cold Word = %q, want fallback", got)
	}
	if got := s.Sentence(0); got != "fallback sentence with words" {
		t.Errorf("cold Sentence = %q, want fallback", got)
	}
	if p := s.Status().Phase; p != core.CorpusWarming {
		t.Errorf("cold Phase = %q, want %q", p, core.CorpusWarming)
	}

	close(gate) // let the producer goroutine's initial fill proceed

	waitFor(t, 2*time.Second, func() bool { return s.Status().Phase == core.CorpusStreaming })

	if got := s.Word(core.LengthAny, 0); got == "fallback" {
		t.Errorf("warmed Word still returned fallback")
	}
	if got := s.Sentence(0); got == "fallback sentence with words" {
		t.Errorf("warmed Sentence still returned fallback")
	}
}

func TestStreamProducerErrorIsSoft(t *testing.T) {
	fallback := &fixedCorpus{word: "fallback", sentence: "fallback sentence with words"}

	fp := &fakeProducer{fn: func(ctx context.Context, kind Kind, n int, emit func(string)) error {
		return errors.New("ollama unreachable")
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewStream(ctx, fp, "fake", fallback)
	defer s.Close()

	waitFor(t, 2*time.Second, func() bool { return s.Status().Phase == core.CorpusFailed })

	status := s.Status()
	if status.Detail != "ollama unreachable" {
		t.Errorf("Detail = %q, want the error text", status.Detail)
	}

	// Never empty, never panics: falls back to the static bank.
	if got := s.Word(core.LengthAny, 0); got != "fallback" {
		t.Errorf("Word after failure = %q, want fallback", got)
	}
	if got := s.Sentence(0); got != "fallback sentence with words" {
		t.Errorf("Sentence after failure = %q, want fallback", got)
	}
}

func TestStreamRecoversFromFailure(t *testing.T) {
	fallback := &fixedCorpus{word: "fallback", sentence: "fallback sentence with words"}

	var failing atomic.Bool
	failing.Store(true)
	var counter int64
	fp := &fakeProducer{fn: func(ctx context.Context, kind Kind, n int, emit func(string)) error {
		if failing.Load() {
			return errors.New("temporary")
		}
		emitN(kind, n, &counter, emit)
		return nil
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewStream(ctx, fp, "fake", fallback)
	defer s.Close()

	waitFor(t, 2*time.Second, func() bool { return s.Status().Phase == core.CorpusFailed })

	failing.Store(false)
	s.signal() // demand-drive a retry instead of waiting out the backoff timer

	waitFor(t, 35*time.Second, func() bool { return s.Status().Phase == core.CorpusStreaming })
}

func TestStreamDemandDrivenRefill(t *testing.T) {
	fallback := &fixedCorpus{word: "fallback", sentence: "fallback sentence with words"}

	var counter int64
	fp := &fakeProducer{fn: func(ctx context.Context, kind Kind, n int, emit func(string)) error {
		emitN(kind, n, &counter, emit)
		return nil
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewStream(ctx, fp, "fake", fallback)
	defer s.Close()

	// Initial cold-start fill: one Produce call each for words and sentences.
	waitFor(t, 2*time.Second, func() bool { return fp.callCount() >= 2 })
	settled := fp.callCount()

	// Plenty of backlog (well above the low-water mark): serving from it
	// must not re-trigger the producer.
	for i := 0; i < serveTopUpThreshold-1; i++ {
		s.Word(core.LengthAny, i)
	}
	time.Sleep(100 * time.Millisecond) // give any (incorrect) extra call a chance to land
	if got := fp.callCount(); got != settled {
		t.Fatalf("Produce called again before threshold: calls = %d, want %d", got, settled)
	}

	// One more serve crosses serveTopUpThreshold and must wake the producer.
	s.Word(core.LengthAny, 9999)
	waitFor(t, 2*time.Second, func() bool { return fp.callCount() > settled })
}

// TestStreamRepetitiveProducerBacksOff is a regression test for a producer
// that keeps returning content the buffer already holds — a real risk with a
// small model asked for hundreds of distinct words.
//
// Because the ring dedups, such a producer can never lift the buffer to its
// low-water mark. Word/Sentence therefore signal "still cold" on every single
// drill item, so unless an unproductive round is penalised exactly like a
// failed one, the producer is re-woken forever and regenerates in a tight loop
// at full GPU — the opposite of the demand-driven contract.
func TestStreamRepetitiveProducerBacksOff(t *testing.T) {
	fallback := &fixedCorpus{word: "fallback", sentence: "fallback sentence with words"}

	// Always emits the same few items: productive once, then never again.
	fp := &fakeProducer{fn: func(ctx context.Context, kind Kind, n int, emit func(string)) error {
		for i := 0; i < 5; i++ {
			if kind == KindWord {
				emit(fmt.Sprintf("same%d", i))
			} else {
				emit(fmt.Sprintf("the same sentence number %d right here", i))
			}
		}
		return nil
	}}

	s := NewStream(context.Background(), fp, "fake", fallback)
	defer s.Close()

	// Cold-start rounds land (5 words, 5 sentences — far below low-water).
	waitFor(t, 2*time.Second, func() bool { return s.Status().Phase == core.CorpusStreaming })
	waitFor(t, 2*time.Second, func() bool { return fp.callCount() >= 2 })
	time.Sleep(100 * time.Millisecond) // let the first unproductive round, if any, settle
	settled := fp.callCount()

	// Hammer it with demand. Every one of these serves sees a below-low-water
	// buffer and signals the producer; the backoff must absorb them all.
	for i := 0; i < 500; i++ {
		s.Word(core.LengthAny, i)
		s.Sentence(i)
	}
	time.Sleep(500 * time.Millisecond) // << minBackoff, so no retry may fire

	// At most one more round (one Produce per kind) may have been in flight.
	if got := fp.callCount(); got > settled+2 {
		t.Fatalf("repetitive producer kept regenerating: %d Produce calls after 1000 serves (was %d); "+
			"unproductive rounds are not backing off", got, settled)
	}

	// It must still serve the content it did manage to get — backing off is
	// not the same as giving up.
	if w := s.Word(core.LengthAny, 0); w == "fallback" {
		t.Fatalf("Word fell back to the static corpus despite having buffered content")
	}
}

func TestStreamCloseIdempotent(t *testing.T) {
	fallback := &fixedCorpus{word: "fallback", sentence: "fallback sentence with words"}
	fp := &fakeProducer{}

	s := NewStream(context.Background(), fp, "fake", fallback)
	s.Close()
	s.Close() // must not hang or panic
}

func TestStreamLengthFilterAndSeed(t *testing.T) {
	fallback := &fixedCorpus{word: "fallback", sentence: "fallback sentence with words"}
	words := []string{"cat", "dog", "elephant", "giraffe"}

	fp := &fakeProducer{fn: func(ctx context.Context, kind Kind, n int, emit func(string)) error {
		if kind == KindWord {
			for _, w := range words {
				emit(w)
			}
		}
		return nil
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewStream(ctx, fp, "fake", fallback)
	defer s.Close()

	waitFor(t, 2*time.Second, func() bool { return s.Status().Words > 0 })

	if got := s.Word(core.LengthShort, 0); len(got) > 4 {
		t.Errorf("LengthShort = %q, want len <= 4", got)
	}
	if got := s.Word(core.LengthLong, 0); len(got) < 6 {
		t.Errorf("LengthLong = %q, want len >= 6", got)
	}
}

func TestRingEvictsOldest(t *testing.T) {
	r := newRing(3)
	for _, w := range []string{"a", "b", "c", "d", "e"} {
		r.add(w)
	}
	if r.len() != 3 {
		t.Fatalf("len = %d, want 3", r.len())
	}
	got := r.snapshot()
	want := []string{"c", "d", "e"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("snapshot = %#v, want %#v", got, want)
	}
}

func TestRingDedup(t *testing.T) {
	r := newRing(5)
	for i := 0; i < 10; i++ {
		if added := r.add("x"); added != (i == 0) {
			t.Errorf("add(%d) = %v, want %v", i, added, i == 0)
		}
	}
	if r.len() != 1 {
		t.Fatalf("len = %d, want 1 (dedup)", r.len())
	}
}

func TestRingDedupSetShrinksOnEviction(t *testing.T) {
	r := newRing(2)
	r.add("a")
	r.add("b")
	r.add("c") // capacity 2: evicts "a"

	if len(r.seen) != 2 {
		t.Fatalf("dedup set size = %d, want 2 (evicted entry removed)", len(r.seen))
	}
	if !r.add("a") {
		t.Errorf("expected re-adding an evicted item to succeed")
	}
}

func TestStreamImplementsCorpusAndStatusReporter(t *testing.T) {
	var (
		_ core.Corpus         = (*Stream)(nil)
		_ core.StatusReporter = (*Stream)(nil)
	)
}
