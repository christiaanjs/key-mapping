package corpus

import (
	"sync"

	"github.com/christiaanswanepoel/key-mapping/core"
)

// Deferred is a core.Corpus whose real source is decided asynchronously. It
// serves a fallback immediately and swaps the real source in when (if) one
// becomes available.
//
// This exists because deciding the corpus can require I/O — probing for a local
// Ollama — and that must not happen before the frontend is usable. In wasm the
// consequence is not merely a slow start but a dead page: Go's main() runs until
// it blocks, and go.run() hands control back to JS at that moment. Doing a fetch
// in main() before js.Global().Set("snapshot", ...) means the page calls
// snapshot() before it exists ("window.snapshot is not a function"). Registering
// the globals first and resolving the corpus behind them is the only ordering
// that works.
//
// While resolution is in flight, Status reports the probe as CorpusWarming, so
// the frontends keep polling and will notice the upgrade. If it fails, Deferred
// settles onto the fallback and stops claiming anything is coming.
type Deferred struct {
	mu      sync.RWMutex
	inner   core.Corpus // never nil
	pending bool
	source  string // what is being probed, for the status line
}

var (
	_ core.Corpus         = (*Deferred)(nil)
	_ core.StatusReporter = (*Deferred)(nil)
)

// NewDeferred returns a Corpus serving fallback until Resolve or Abandon is
// called. source names what is being probed (e.g. "ollama") for the status line.
func NewDeferred(fallback core.Corpus, source string) *Deferred {
	return &Deferred{inner: fallback, pending: true, source: source}
}

// Resolve swaps in the real corpus. Safe to call from another goroutine while
// the frontend is reading.
func (d *Deferred) Resolve(c core.Corpus) {
	if c == nil {
		d.Abandon()
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.inner = c
	d.pending = false
}

// Abandon gives up on the real corpus and settles on the fallback.
func (d *Deferred) Abandon() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pending = false
}

func (d *Deferred) Word(length core.Length, seed int) string {
	d.mu.RLock()
	inner := d.inner
	d.mu.RUnlock()
	return inner.Word(length, seed)
}

func (d *Deferred) Sentence(seed int) string {
	d.mu.RLock()
	inner := d.inner
	d.mu.RUnlock()
	return inner.Sentence(seed)
}

// Status reports the probe while it is in flight, and otherwise whatever the
// resolved corpus says (or a plain static bank, if it has nothing to say).
func (d *Deferred) Status() core.CorpusStatus {
	d.mu.RLock()
	inner, pending, source := d.inner, d.pending, d.source
	d.mu.RUnlock()

	if pending {
		return core.CorpusStatus{Source: source, Phase: core.CorpusWarming}
	}
	if r, ok := inner.(core.StatusReporter); ok {
		return r.Status()
	}
	return core.CorpusStatus{Source: "static", Phase: core.CorpusReady}
}
