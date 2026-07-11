package corpus

import "context"

// Kind distinguishes the two things a producer can be asked for.
type Kind int

const (
	KindWord Kind = iota
	KindSentence
)

// Producer streams practice text from some generator. Produce asks for about
// n items of the given kind and calls emit with each one as soon as it is
// complete — not batched at the end — so a caller (Stream) can make partial
// progress usable well before the whole batch finishes. It returns when that
// batch is done, or on error. Implementations must respect ctx cancellation.
//
// This interface is deliberately provider-agnostic: today only Ollama
// implements it (ollama.go); a later phase adds an Anthropic-backed
// implementation behind the same seam.
type Producer interface {
	Produce(ctx context.Context, kind Kind, n int, emit func(string)) error
}
