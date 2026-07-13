package corpus

import (
	"testing"

	"github.com/christiaanswanepoel/key-mapping/core"
)

// TestDeferredServesFallbackUntilResolved covers the ordering that keeps the
// page alive: the corpus must be usable the instant it is constructed, long
// before anything is known about a remote model.
func TestDeferredServesFallbackUntilResolved(t *testing.T) {
	fallback := &fixedCorpus{word: "fallback", sentence: "fallback sentence with words"}
	d := NewDeferred(fallback, "ollama")

	if got := d.Word(core.LengthAny, 0); got != "fallback" {
		t.Errorf("Word before resolution = %q, want the fallback", got)
	}
	if got := d.Sentence(0); got != fallback.sentence {
		t.Errorf("Sentence before resolution = %q, want the fallback", got)
	}

	// While probing, it must NOT look settled: the frontends stop polling on a
	// "ready" phase, and would then never notice the upgrade.
	st := d.Status()
	if st.Phase != core.CorpusWarming || st.Source != "ollama" {
		t.Errorf("Status while pending = %+v, want warming/ollama", st)
	}

	real := &fixedCorpus{word: "generated", sentence: "a generated sentence right here"}
	d.Resolve(real)

	if got := d.Word(core.LengthAny, 0); got != "generated" {
		t.Errorf("Word after resolution = %q, want the resolved corpus", got)
	}
	if got := d.Sentence(0); got != real.sentence {
		t.Errorf("Sentence after resolution = %q, want the resolved corpus", got)
	}
}

func TestDeferredAbandonSettlesOnFallback(t *testing.T) {
	fallback := &fixedCorpus{word: "fallback", sentence: "fallback sentence with words"}
	d := NewDeferred(fallback, "ollama")

	d.Abandon()

	// Settled: no Ollama, so the frontends should stop polling and stop
	// implying that something is on its way.
	st := d.Status()
	if st.Phase != core.CorpusReady || st.Source != "static" {
		t.Errorf("Status after Abandon = %+v, want a ready static bank", st)
	}
	if got := d.Word(core.LengthAny, 0); got != "fallback" {
		t.Errorf("Word after Abandon = %q, want the fallback", got)
	}
}

// TestDeferredReportsResolvedStatus checks the status delegates once resolved,
// so a Stream's live phase and counts reach the frontends through the wrapper.
func TestDeferredReportsResolvedStatus(t *testing.T) {
	d := NewDeferred(&fixedCorpus{word: "fallback", sentence: "fallback sentence here"}, "ollama")

	fp := &fakeProducer{}
	s := NewStream(t.Context(), fp, "ollama", &fixedCorpus{word: "fb", sentence: "fb sentence with words"})
	defer s.Close()

	d.Resolve(s)

	if got := d.Status().Source; got != "ollama" {
		t.Errorf("Status().Source after resolving a Stream = %q, want the Stream's own", got)
	}
}

func TestDeferredResolveNilAbandons(t *testing.T) {
	d := NewDeferred(&fixedCorpus{word: "fallback", sentence: "fallback sentence here"}, "ollama")

	d.Resolve(nil) // must not panic, and must not leave a nil inner corpus

	if got := d.Word(core.LengthAny, 0); got != "fallback" {
		t.Errorf("Word after Resolve(nil) = %q, want the fallback", got)
	}
	if st := d.Status(); st.Phase != core.CorpusReady {
		t.Errorf("Status after Resolve(nil) = %+v, want settled", st)
	}
}
