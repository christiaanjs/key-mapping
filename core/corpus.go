package core

// Corpus supplies practice text for the drill. It is the seam that decouples the
// trainer from where content comes from: a hardcoded bank (staticCorpus), a text
// file, a codebase, or a language model streaming text in as you type — the drill
// never knows which.
//
// The core is pure and holds no random source, so selection is seeded by the
// caller: the App passes an incrementing counter (one increment per drill item)
// as seed, and calls Word or Sentence exactly once per item.
//
// Implementations backed by a fixed bank must be deterministic in seed (same
// seed -> same result) so drills are testable. A streaming implementation cannot
// be — its bank grows as content arrives — and does not need to be: the App never
// re-reads an old seed. What every implementation must be is **non-blocking**:
// Word and Sentence are called on the frontend's event loop (Bubble Tea's Update,
// the browser's single JS thread), so a source that fetches content must do it in
// the background and serve whatever it has, never wait for it.
type Corpus interface {
	// Word returns a practice word matching the length filter, chosen by seed.
	Word(length Length, seed int) string

	// Sentence returns a practice sentence, chosen by seed.
	Sentence(seed int) string
}

// StatusReporter is an optional interface a Corpus may implement to describe
// where its content comes from and whether more is still arriving. App.Snapshot
// surfaces it as State.Corpus, so frontends can show the user that a streaming
// source is warming up, how much has landed, or that it fell back.
//
// A Corpus that does not implement it is reported as a plain, ready bank.
type StatusReporter interface {
	Status() CorpusStatus
}

// corpusStatus reports the Corpus's own status if it implements StatusReporter,
// and a ready-bank default otherwise.
//
// A streaming Corpus is mutated by a background goroutine while the App reads
// it, so its Status (and Word/Sentence) must be internally synchronized. That
// concurrency lives entirely inside the Corpus implementation — the App itself
// remains single-threaded and unlocked, per the architecture doc.
func (a *App) corpusStatus() CorpusStatus {
	if r, ok := a.corpus.(StatusReporter); ok {
		return r.Status()
	}
	return CorpusStatus{Source: "static", Phase: CorpusReady}
}
