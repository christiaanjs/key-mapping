package corpus

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/christiaanswanepoel/key-mapping/core"
)

// Buffer sizes and refill thresholds. Content is unbounded (a streaming
// source keeps generating forever); memory is not, so each kind's ring
// caps out and starts evicting its oldest entries.
const (
	wordCap     = 2000
	sentenceCap = 500

	// Low-water marks: below these, the buffer is "cold" and the producer
	// tops up urgently (this is also what cold-starts the whole stream).
	wordLowWater     = 200
	sentenceLowWater = 50

	// Once above the low-water mark, top up again only after this many
	// Word/Sentence calls have been served for that kind since the last
	// top-up attempt — demand-driven, not time-driven.
	serveTopUpThreshold = 100

	// Amount requested by a demand-triggered top-up (as opposed to the
	// cold-start fill, which requests up to the low-water mark).
	wordTopUpAmount     = wordLowWater
	sentenceTopUpAmount = sentenceLowWater

	minBackoff = 2 * time.Second
	maxBackoff = 30 * time.Second
)

// Describer is optionally implemented by a Producer to supply a
// human-readable default for CorpusStatus.Detail (e.g. the model name)
// while generation is healthy. A Producer that doesn't implement it is
// reported with no detail until something fails.
type Describer interface {
	Describe() string
}

// Stream is a core.Corpus backed by a Producer that generates content in the
// background. It never blocks the caller: Word/Sentence always return
// immediately from whatever is currently buffered, falling back to a static
// Corpus while a kind's buffer is empty. A single goroutine refills each
// buffer on demand — see the low/high-water constants above — and is woken
// via a non-blocking signal rather than polling.
type Stream struct {
	producer Producer
	fallback core.Corpus
	source   string

	ctx       context.Context
	cancel    context.CancelFunc
	wake      chan struct{}
	done      chan struct{}
	closeOnce sync.Once

	// mu guards everything below. Word/Sentence take it briefly (a slice
	// copy under lock, no I/O) to read the buffer and bump serve counters,
	// so this never blocks the frontend event loop that calls them.
	mu                       sync.Mutex
	words                    *ring
	sentences                *ring
	wordServesSinceTopUp     int
	sentenceServesSinceTopUp int
	phase                    core.CorpusPhase
	detail                   string
	backoff                  time.Duration
	nextAttempt              time.Time
}

var (
	_ core.Corpus         = (*Stream)(nil)
	_ core.StatusReporter = (*Stream)(nil)
)

// NewStream starts a background producer goroutine and returns immediately;
// callers get a usable Corpus before any content has generated because
// Word/Sentence serve fallback until the buffers warm up. Cancelling ctx (or
// calling Close) stops the goroutine.
func NewStream(ctx context.Context, p Producer, name string, fallback core.Corpus) *Stream {
	cctx, cancel := context.WithCancel(ctx)
	s := &Stream{
		producer:  p,
		fallback:  fallback,
		source:    name,
		ctx:       cctx,
		cancel:    cancel,
		wake:      make(chan struct{}, 1),
		done:      make(chan struct{}),
		words:     newRing(wordCap),
		sentences: newRing(sentenceCap),
		phase:     core.CorpusWarming,
		backoff:   minBackoff,
	}
	go s.run()
	return s
}

// Close stops the producer goroutine and waits for it to exit. Idempotent
// and safe to call twice (or not at all — cancelling the ctx passed to
// NewStream has the same effect).
func (s *Stream) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
	})
	<-s.done
}

// Word returns a buffered generated word matching the length filter, or
// falls back while the word buffer is empty. Never blocks.
func (s *Stream) Word(length core.Length, seed int) string {
	s.mu.Lock()
	words := s.words.snapshot()
	s.wordServesSinceTopUp++
	needsWake := s.words.len() < wordLowWater || s.wordServesSinceTopUp >= serveTopUpThreshold
	s.mu.Unlock()

	if needsWake {
		s.signal()
	}
	if len(words) == 0 {
		return s.fallback.Word(length, seed)
	}
	return selectWord(words, length, seed)
}

// Sentence returns a buffered generated sentence, or falls back while the
// sentence buffer is empty. Never blocks.
func (s *Stream) Sentence(seed int) string {
	s.mu.Lock()
	sentences := s.sentences.snapshot()
	s.sentenceServesSinceTopUp++
	needsWake := s.sentences.len() < sentenceLowWater || s.sentenceServesSinceTopUp >= serveTopUpThreshold
	s.mu.Unlock()

	if needsWake {
		s.signal()
	}
	if len(sentences) == 0 {
		return s.fallback.Sentence(seed)
	}
	return selectSentence(sentences, seed)
}

// Status reports the stream's current phase and buffer sizes. See
// core.CorpusStatus for field meaning.
func (s *Stream) Status() core.CorpusStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	detail := s.detail
	if s.phase != core.CorpusFailed {
		if d, ok := s.producer.(Describer); ok {
			detail = d.Describe()
		} else {
			detail = ""
		}
	}

	return core.CorpusStatus{
		Source:    s.source,
		Phase:     s.phase,
		Words:     s.words.len(),
		Sentences: s.sentences.len(),
		Detail:    detail,
	}
}

// signal wakes the producer goroutine without blocking. The channel is
// buffered by one, so a signal already pending (goroutine busy, or hasn't
// woken yet) is enough — extra sends are no-ops rather than piling up.
func (s *Stream) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// run is the producer goroutine: it fills the buffers once at startup (the
// cold-start case), then sleeps until either woken by demand (signal) or a
// backoff timer from a prior error expires. It never busy-polls.
func (s *Stream) run() {
	defer close(s.done)

	s.produceRound()
	for {
		s.mu.Lock()
		wait := time.Until(s.nextAttempt)
		s.mu.Unlock()

		if wait > 0 {
			// Inside a backoff window (the last round failed or added nothing
			// new). Demand signals are deliberately NOT selected on here: the
			// whole point of the backoff is to stop a failing or repetitive
			// producer from being re-triggered on every drill item. Honouring a
			// wake here would let a dead server be hammered once per keystroke.
			timer := time.NewTimer(wait)
			select {
			case <-s.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		} else {
			select {
			case <-s.ctx.Done():
				return
			case <-s.wake:
			}
		}

		s.produceRound()
	}
}

// produceRound asks the producer to top up whichever kinds need it. Called
// only from run, so it owns no lock itself beyond what evaluateNeeds and
// produceKind each take.
func (s *Stream) produceRound() {
	wNeed, sNeed := s.evaluateNeeds()
	if wNeed > 0 {
		s.produceKind(KindWord, wNeed)
	}
	if sNeed > 0 {
		s.produceKind(KindSentence, sNeed)
	}
}

// evaluateNeeds decides how many words/sentences to request: enough to
// reach the low-water mark if below it (cold start or a deep dip), or a
// fixed top-up amount once enough calls have been served since the last
// attempt. Zero means "nothing to do right now".
func (s *Stream) evaluateNeeds() (wNeed, sNeed int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if n := s.words.len(); n < wordLowWater {
		wNeed = wordLowWater - n
	} else if s.wordServesSinceTopUp >= serveTopUpThreshold {
		wNeed = wordTopUpAmount
	}

	if n := s.sentences.len(); n < sentenceLowWater {
		sNeed = sentenceLowWater - n
	} else if s.sentenceServesSinceTopUp >= serveTopUpThreshold {
		sNeed = sentenceTopUpAmount
	}

	return wNeed, sNeed
}

// produceKind runs one Produce call for kind and folds the result (or error)
// into phase/backoff state. A failure is soft: it never removes
// already-buffered content, it just marks the phase and schedules a retry.
//
// An *unproductive* round — one that succeeds but adds nothing new, because
// the model repeated words we already hold — backs off exactly like a failure
// does. Without that, a model that cannot produce wordLowWater distinct items
// would leave the buffer permanently below the mark, so every drill item would
// re-signal and the producer would regenerate forever at full GPU. Backing off
// is what keeps "idle => no generation" true even for a repetitive model.
func (s *Stream) produceKind(kind Kind, n int) {
	s.mu.Lock()
	if kind == KindWord {
		s.wordServesSinceTopUp = 0
	} else {
		s.sentenceServesSinceTopUp = 0
	}
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
			// Shutting down; not a generation failure worth reporting.
			return
		}
		s.phase = core.CorpusFailed
		s.detail = err.Error()
		s.penalizeLocked()
		return
	}

	if s.words.len() > 0 || s.sentences.len() > 0 {
		s.phase = core.CorpusStreaming
	}

	if atomic.LoadInt64(&added) == 0 {
		s.penalizeLocked()
		return
	}

	// Real progress: clear any backoff so a cold buffer can keep filling at
	// full speed until it reaches its low-water mark.
	s.backoff = minBackoff
	s.nextAttempt = time.Time{}
}

// penalizeLocked schedules the next attempt after an exponentially growing
// backoff, capped at maxBackoff. Caller must hold s.mu.
func (s *Stream) penalizeLocked() {
	if s.backoff < minBackoff {
		s.backoff = minBackoff
	}
	s.nextAttempt = time.Now().Add(s.backoff)
	s.backoff *= 2
	if s.backoff > maxBackoff {
		s.backoff = maxBackoff
	}
}

// ingest adds one produced item to the right ring, deduplicating and
// evicting per ring semantics, and flips warming -> streaming the moment
// the very first item (of either kind) lands — mid-batch, not at the end of
// the Produce call, so the drill can switch off the fallback as soon as
// there is something real to serve.
//
// It reports whether the item was actually new. produceKind counts those: a
// round that adds nothing is what tells the stream the model is repeating
// itself and generation should back off.
func (s *Stream) ingest(kind Kind, item string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	var added bool
	switch kind {
	case KindWord:
		added = s.words.add(item)
	case KindSentence:
		added = s.sentences.add(item)
	}

	if s.phase == core.CorpusWarming && (s.words.len() > 0 || s.sentences.len() > 0) {
		s.phase = core.CorpusStreaming
	}
	return added
}

// ring is a bounded, deduplicated FIFO of strings: once full, adding a new
// item overwrites the oldest, and the dedup set is kept in sync with that
// eviction so it cannot grow without bound alongside unbounded content.
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

// add inserts item unless it is empty or already present. Reports whether
// it was added.
func (r *ring) add(item string) bool {
	if item == "" || len(r.items) == 0 || r.seen[item] {
		return false
	}

	if r.size == len(r.items) {
		oldest := r.items[r.head]
		delete(r.seen, oldest)
		r.items[r.head] = item
		r.head = (r.head + 1) % len(r.items)
	} else {
		idx := (r.head + r.size) % len(r.items)
		r.items[idx] = item
		r.size++
	}
	r.seen[item] = true
	return true
}

func (r *ring) len() int { return r.size }

// snapshot copies out the ring's current contents, oldest first, so the
// caller can use them without holding Stream's lock.
func (r *ring) snapshot() []string {
	out := make([]string, r.size)
	for i := 0; i < r.size; i++ {
		out[i] = r.items[(r.head+i)%len(r.items)]
	}
	return out
}
