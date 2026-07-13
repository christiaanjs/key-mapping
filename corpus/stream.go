package corpus

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/christiaanswanepoel/key-mapping/core"
)

// Buffer sizes and refill thresholds. Content is unbounded (a streaming source
// keeps generating for as long as the drill wants more); memory is not, so each
// kind's ring caps out and starts evicting its oldest entries.
const (
	wordCap     = 2000
	sentenceCap = 500

	// Low-water marks: below these a buffer is "cold" and its producer tops up
	// urgently (this is also what cold-starts the stream). They double as the
	// size of a single request, so they are kept modest: a model asked for a
	// large batch streams for minutes, and nothing else about that kind can
	// make progress until it finishes.
	wordLowWater     = 60
	sentenceLowWater = 20

	// Once above the low-water mark, a kind earns a top-up after the drill has
	// drawn from it about as many times as it holds — i.e. one full pass through
	// everything it has. Demand-driven, not time-driven.
	//
	// This scales with the bank rather than being a fixed count, and that matters:
	// a flat threshold (it was 100) meant the 20-sentence bank had to be cycled
	// FIVE times — every sentence seen five times over — before a single new one
	// was generated. Skipping felt like it did nothing, because it did nothing.
	// Tying it to the bank size also self-throttles: as the ring grows toward its
	// cap, top-ups naturally become rarer.
	//
	// The floor stops a nearly-empty bank from re-triggering on every draw.
	minServesBeforeTopUp = 10

	minBackoff = 2 * time.Second
	maxBackoff = 30 * time.Second
)

// Describer is optionally implemented by a Producer to supply a human-readable
// default for CorpusStatus.Detail (e.g. the model name) while generation is
// healthy. A Producer that doesn't implement it is reported with no detail
// until something fails.
type Describer interface {
	Describe() string
}

// Stream is a core.Corpus backed by a Producer that generates content in the
// background. It never blocks the caller: Word/Sentence always return
// immediately from whatever is currently buffered, falling back to a static
// Corpus while a kind's buffer is empty.
//
// Each kind gets its OWN producer goroutine and its own buffer, backoff and
// wake signal. That is not incidental: a Produce call runs to completion, and a
// model asked for a batch of words can stream for minutes. With a single
// producer, the sentence buffer would be starved for that whole time — observed
// live as sentences stuck at 0 for over two minutes while words trickled in.
// Independent loops mean neither kind can block the other.
type Stream struct {
	producer Producer
	fallback core.Corpus
	source   string

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once

	// mu guards everything below, and is held only for cheap in-memory work —
	// never across a Produce call — so Word/Sentence (which run on the
	// frontend's event loop) are never made to wait on the network.
	mu       sync.Mutex
	words    *kindState
	sentence *kindState
	anyItem  bool   // has anything at all ever been generated?
	lastErr  string // last produce error; cleared by the next success
}

// kindState is everything a single kind (words or sentences) needs to keep
// itself stocked, independently of the other.
type kindState struct {
	ring             *ring
	servesSinceTopUp int
	backoff          time.Duration
	nextAttempt      time.Time
	wake             chan struct{}

	lowWater int
}

var (
	_ core.Corpus         = (*Stream)(nil)
	_ core.StatusReporter = (*Stream)(nil)
)

// NewStream starts the background producers and returns immediately; callers
// get a usable Corpus before any content has been generated, because
// Word/Sentence serve the fallback until the buffers warm up. Cancelling ctx
// (or calling Close) stops the producers.
func NewStream(ctx context.Context, p Producer, name string, fallback core.Corpus) *Stream {
	cctx, cancel := context.WithCancel(ctx)
	s := &Stream{
		producer: p,
		fallback: fallback,
		source:   name,
		ctx:      cctx,
		cancel:   cancel,
		words:    newKindState(wordCap, wordLowWater),
		sentence: newKindState(sentenceCap, sentenceLowWater),
	}

	s.wg.Add(2)
	go s.run(KindWord)
	go s.run(KindSentence)
	return s
}

func newKindState(capacity, lowWater int) *kindState {
	return &kindState{
		ring:     newRing(capacity),
		backoff:  minBackoff,
		wake:     make(chan struct{}, 1),
		lowWater: lowWater,
	}
}

// state returns the kindState for kind. Caller need not hold mu: the pointer
// itself is immutable after construction.
func (s *Stream) state(kind Kind) *kindState {
	if kind == KindSentence {
		return s.sentence
	}
	return s.words
}

// Close stops the producer goroutines and waits for them to exit. Idempotent
// and safe to call twice (or not at all — cancelling the ctx passed to
// NewStream has the same effect).
func (s *Stream) Close() {
	s.closeOnce.Do(func() { s.cancel() })
	s.wg.Wait()
}

// Word returns a buffered generated word matching the length filter, or falls
// back while the word buffer is empty. Never blocks.
func (s *Stream) Word(length core.Length, seed int) string {
	words, needsWake := s.serve(KindWord)
	if needsWake {
		s.signal(KindWord)
	}
	if len(words) == 0 {
		return s.fallback.Word(length, seed)
	}
	return selectWord(words, length, seed)
}

// Sentence returns a buffered generated sentence, or falls back while the
// sentence buffer is empty. Never blocks.
func (s *Stream) Sentence(seed int) string {
	sentences, needsWake := s.serve(KindSentence)
	if needsWake {
		s.signal(KindSentence)
	}
	if len(sentences) == 0 {
		return s.fallback.Sentence(seed)
	}
	return selectSentence(sentences, seed)
}

// serve copies out a kind's current items and records the draw, reporting
// whether that draw means the producer should be woken.
func (s *Stream) serve(kind Kind) (items []string, needsWake bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.state(kind)
	st.servesSinceTopUp++
	return st.ring.snapshot(), st.ring.len() < st.lowWater || st.topUpDue()
}

// Status reports the stream's phase and buffer sizes. See core.CorpusStatus.
func (s *Stream) Status() core.CorpusStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	// A failure is reported even once content is buffered: generation being
	// broken is worth telling the user about, whether or not they can still
	// drill. The next successful round clears it.
	var phase core.CorpusPhase
	var detail string
	switch {
	case s.lastErr != "":
		phase, detail = core.CorpusFailed, s.lastErr
	case !s.anyItem:
		phase, detail = core.CorpusWarming, s.describe()
	default:
		phase, detail = core.CorpusStreaming, s.describe()
	}

	return core.CorpusStatus{
		Source:    s.source,
		Phase:     phase,
		Words:     s.words.ring.len(),
		Sentences: s.sentence.ring.len(),
		Detail:    detail,
	}
}

// describe reports the producer's self-description (e.g. the model name), if it
// offers one.
func (s *Stream) describe() string {
	if d, ok := s.producer.(Describer); ok {
		return d.Describe()
	}
	return ""
}

// signal wakes a kind's producer without blocking. The channel is buffered by
// one, so a signal already pending is enough — extra sends are no-ops rather
// than piling up.
func (s *Stream) signal(kind Kind) {
	select {
	case s.state(kind).wake <- struct{}{}:
	default:
	}
}

// run is one kind's producer goroutine. Each iteration either waits out a
// backoff, sleeps until demand wakes it, or runs one round — so it generates
// while there is something to generate and is otherwise completely idle. It
// never busy-polls.
func (s *Stream) run(kind Kind) {
	defer s.wg.Done()

	st := s.state(kind)

	for {
		s.mu.Lock()
		wait := time.Until(st.nextAttempt)
		s.mu.Unlock()

		if wait > 0 {
			// Inside a backoff window (the last round failed, or added nothing
			// new). Demand signals are deliberately NOT selected on here: the
			// point of the backoff is to stop a failing or repetitive producer
			// from being re-triggered on every drill item. Honouring a wake
			// would let a dead server be hammered once per keystroke.
			timer := time.NewTimer(wait)
			select {
			case <-s.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		} else if s.need(kind) == 0 {
			// Nothing to generate: sleep until the drill draws enough to need
			// more. This is what keeps an idle trainer at zero cost.
			select {
			case <-s.ctx.Done():
				return
			case <-st.wake:
			}
		}
		// Otherwise fall straight through: a cold buffer keeps filling without
		// waiting for a keystroke.

		if n := s.need(kind); n > 0 {
			s.produce(kind, n)
		}
		if s.ctx.Err() != nil {
			return
		}
	}
}

// need reports how many items of kind to request: enough to reach the low-water
// mark if below it (cold start, or a deep dip), or a fresh batch once the drill
// has drawn enough since the last top-up. Zero means "nothing to do".
func (s *Stream) need(kind Kind) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.state(kind)
	if deficit := st.lowWater - st.ring.len(); deficit > 0 {
		return deficit
	}
	if st.topUpDue() {
		return st.lowWater
	}
	return 0
}

// topUpDue reports whether the drill has drawn from this kind enough times since
// its last top-up to have earned fresh material: roughly one full pass through
// what it currently holds. Caller must hold Stream.mu.
func (st *kindState) topUpDue() bool {
	threshold := st.ring.len()
	if threshold < minServesBeforeTopUp {
		threshold = minServesBeforeTopUp
	}
	return st.servesSinceTopUp >= threshold
}

// produce runs one Produce call for kind and folds the result into phase and
// backoff state. A failure is soft: it never discards buffered content, it just
// records the error and schedules a retry.
//
// An *unproductive* round — one that succeeds but adds nothing new, because the
// model repeated items we already hold — backs off exactly like a failure.
// Without that, a model that cannot produce lowWater distinct items would leave
// the buffer permanently below the mark, so every drill item would re-signal
// and the producer would regenerate forever at full GPU. This is what keeps
// "idle => no generation" true even for a repetitive model.
func (s *Stream) produce(kind Kind, n int) {
	s.mu.Lock()
	s.state(kind).servesSinceTopUp = 0
	s.mu.Unlock()

	// emit may be called from whatever goroutine the producer streams on, so
	// count atomically rather than assuming Produce is single-threaded.
	var added int64
	err := s.producer.Produce(s.ctx, kind, n, func(item string) {
		if s.ingest(kind, item) {
			atomic.AddInt64(&added, 1)
		}
	})

	s.mu.Lock()
	defer s.mu.Unlock()

	if err != nil {
		if s.ctx.Err() != nil {
			return // shutting down; not a generation failure worth reporting
		}
		s.lastErr = err.Error()
		s.penalizeLocked(kind)
		return
	}

	s.lastErr = ""

	if atomic.LoadInt64(&added) == 0 {
		s.penalizeLocked(kind)
		return
	}

	// Real progress: clear the backoff so a cold buffer keeps filling at full
	// speed until it reaches its low-water mark.
	st := s.state(kind)
	st.backoff = minBackoff
	st.nextAttempt = time.Time{}
}

// penalizeLocked schedules kind's next attempt after an exponentially growing
// backoff, capped at maxBackoff. Caller must hold s.mu.
func (s *Stream) penalizeLocked(kind Kind) {
	st := s.state(kind)
	if st.backoff < minBackoff {
		st.backoff = minBackoff
	}
	st.nextAttempt = time.Now().Add(st.backoff)
	st.backoff *= 2
	if st.backoff > maxBackoff {
		st.backoff = maxBackoff
	}
}

// ingest adds one produced item to its ring, deduplicating and evicting per
// ring semantics, and records that the stream has produced something — which
// flips warming -> streaming the moment the very first item lands, mid-batch
// rather than at the end of the Produce call, so the drill stops serving
// fallback text as soon as there is something real.
//
// It reports whether the item was actually new; produce counts those to detect
// a model that is repeating itself.
func (s *Stream) ingest(kind Kind, item string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	added := s.state(kind).ring.add(item)
	if added {
		s.anyItem = true
	}
	return added
}

// ring is a bounded, deduplicated FIFO of strings: once full, adding a new item
// overwrites the oldest, and the dedup set is kept in sync with that eviction so
// it cannot grow without bound alongside unbounded content.
type ring struct {
	items []string
	seen  map[string]bool
	head  int // index of the oldest item
	size  int // number of valid items, <= len(items)
}

func newRing(capacity int) *ring {
	return &ring{
		items: make([]string, capacity),
		seen:  make(map[string]bool, capacity),
	}
}

// add inserts item unless it is empty or already present. Reports whether it
// was added.
func (r *ring) add(item string) bool {
	if item == "" || len(r.items) == 0 || r.seen[item] {
		return false
	}

	if r.size == len(r.items) {
		delete(r.seen, r.items[r.head])
		r.items[r.head] = item
		r.head = (r.head + 1) % len(r.items)
	} else {
		r.items[(r.head+r.size)%len(r.items)] = item
		r.size++
	}
	r.seen[item] = true
	return true
}

func (r *ring) len() int { return r.size }

// snapshot copies out the ring's current contents, oldest first, so the caller
// can use them without holding Stream's lock.
func (r *ring) snapshot() []string {
	out := make([]string, r.size)
	for i := 0; i < r.size; i++ {
		out[i] = r.items[(r.head+i)%len(r.items)]
	}
	return out
}
