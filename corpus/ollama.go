package corpus

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ollama/ollama/api"
	"github.com/ollama/ollama/types/model"

	"github.com/christiaanswanepoel/key-mapping/core"
)

// Options configures NewOllama. Zero values pick sensible defaults.
type Options struct {
	// Model is the Ollama model name to query (e.g. "llama3.2"). Defaults to
	// "llama3.2" if empty.
	Model string

	// Host is the Ollama server address (e.g. "http://localhost:11434"). If
	// empty, the client is built from the OLLAMA_HOST environment variable
	// (or its own default) via api.ClientFromEnvironment.
	Host string
}

const defaultModel = "llama3.2"

func (o Options) withDefaults() Options {
	if o.Model == "" {
		o.Model = defaultModel
	}
	return o
}

// ollamaProducer is a Producer backed by a local Ollama server. Unlike the
// old batch FromOllama, each Produce call streams the Generate response and
// emits lines as they complete rather than waiting for the whole thing, so
// the Stream wrapping it can serve real content within a second or two.
type ollamaProducer struct {
	client *api.Client
	model  string

	// round is folded into the prompt (via a rotating theme) so repeated
	// Produce calls ask for different content instead of regenerating the
	// same list. Produce is called from Stream's single producer goroutine,
	// but incremented atomically to be safe against any other caller.
	round int64

	// Whether this model is a "thinking" model, resolved once from the
	// server and cached. See noThink for why this matters.
	thinkMu       sync.Mutex
	thinkResolved bool
	thinks        bool
}

// noThink returns the Think value to send for this model.
//
// A reasoning model (qwen3, deepseek-r1, ...) defaults to emitting a long
// chain-of-thought before it answers. That trace goes to GenerateResponse
// .Thinking, not .Response, so we would sit there for minutes streaming
// nothing usable — the corpus would stay stuck "warming" while the GPU churns.
// We want the word list, not the reasoning, so thinking is explicitly disabled.
//
// It has to be conditional: sending `think` to a model that has no thinking
// capability is rejected by the server, so the field is left unset for those.
// The capability is resolved once, lazily, and a failed lookup is simply
// retried next round rather than being cached as a negative.
func (p *ollamaProducer) noThink(ctx context.Context) *api.ThinkValue {
	p.thinkMu.Lock()
	defer p.thinkMu.Unlock()

	if !p.thinkResolved {
		resp, err := p.client.Show(ctx, &api.ShowRequest{Model: p.model})
		if err != nil {
			// Leave it unresolved: if the server is simply down, Produce is
			// about to fail anyway and Stream will back off and retry.
			return nil
		}
		for _, c := range resp.Capabilities {
			if c == model.CapabilityThinking {
				p.thinks = true
				break
			}
		}
		p.thinkResolved = true
	}

	if !p.thinks {
		return nil
	}
	return &api.ThinkValue{Value: false}
}

// NewOllama builds a Producer backed by opts. It fails only if the client
// itself cannot be constructed (e.g. a bad Host URL); an unreachable server
// is a runtime concern that Stream handles as a soft failure, not something
// this constructor can detect up front.
func NewOllama(opts Options) (Producer, error) {
	opts = opts.withDefaults()

	client, err := newOllamaClient(opts.Host)
	if err != nil {
		return nil, fmt.Errorf("corpus: build ollama client: %w", err)
	}

	return &ollamaProducer{client: client, model: opts.Model}, nil
}

// FromOllama is the convenience entrypoint frontends use: it builds an
// Ollama producer and wraps it in a Stream that falls back to fallback
// whenever a kind's buffer is empty. It returns an error only if the client
// cannot be constructed — never for a down or slow server, which Stream
// handles internally so a dead Ollama never breaks the trainer.
func FromOllama(ctx context.Context, opts Options, fallback core.Corpus) (*Stream, error) {
	p, err := NewOllama(opts)
	if err != nil {
		return nil, err
	}
	return NewStream(ctx, p, "ollama", fallback), nil
}

// Describe reports the model name; Stream surfaces it as CorpusStatus.Detail
// while generation is healthy.
func (p *ollamaProducer) Describe() string { return p.model }

// Produce asks the model for about n items of kind and emits each as soon as
// its line is complete, using the client's streaming mode (Stream is left at
// its default of true, so GenerateResponseFunc fires per chunk rather than
// once at the end).
func (p *ollamaProducer) Produce(ctx context.Context, kind Kind, n int, emit func(string)) error {
	round := atomic.AddInt64(&p.round, 1) - 1
	prompt := promptFor(kind, n, round)

	var pending strings.Builder
	req := &api.GenerateRequest{
		Model:  p.model,
		Prompt: prompt,
		Think:  p.noThink(ctx),
	}

	err := p.client.Generate(ctx, req, func(r api.GenerateResponse) error {
		// r.Thinking holds a "thinking" model's reasoning trace (e.g.
		// qwen3), never the list content itself — only r.Response is real
		// output and must be accumulated.
		for _, c := range r.Response {
			if c == '\n' {
				emitLine(kind, pending.String(), emit)
				pending.Reset()
				continue
			}
			pending.WriteRune(c)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("corpus: ollama generate: %w", err)
	}

	// The response may not end in a newline; flush whatever's left.
	if pending.Len() > 0 {
		emitLine(kind, pending.String(), emit)
	}

	return nil
}

// emitLine cleans one line of model output according to kind (reusing the
// same tokenizers the rest of the package uses) and emits it if it cleans to
// something usable; a junk line emits nothing.
func emitLine(kind Kind, line string, emit func(string)) {
	switch kind {
	case KindWord:
		if w, ok := firstWord(line); ok {
			emit(w)
		}
	case KindSentence:
		if s, ok := cleanSentenceLine(line); ok {
			emit(s)
		}
	}
}

// themes rotate across rounds so successive Produce calls for the same kind
// ask about different subject matter instead of regenerating the same list.
var themes = []string{
	"everyday life", "nature and weather", "food and cooking", "travel and places",
	"work and school", "sports and games", "technology", "animals",
	"family and friends", "emotions", "the seasons", "music and art",
}

// promptFor builds the Generate prompt for n items of kind, folding round
// into a rotating theme. The rules (lowercase a-z only, one item per line,
// no numbering/bullets/commentary) match what the old batch prompts asked
// for; only the theme varies across rounds.
func promptFor(kind Kind, n int, round int64) string {
	theme := themes[int(round%int64(len(themes)))]

	switch kind {
	case KindWord:
		return fmt.Sprintf(
			"List %d common English words related to %s, for a touch-typing practice tool.\n"+
				"Rules:\n"+
				"- lowercase letters a-z only, no punctuation, no numbers, no apostrophes\n"+
				"- one word per line\n"+
				"- no numbering, no bullets, no extra commentary\n"+
				"- no duplicate words\n",
			n, theme,
		)
	default: // KindSentence
		return fmt.Sprintf(
			"Write %d short, simple English sentences about %s, for a touch-typing practice tool.\n"+
				"Rules:\n"+
				"- lowercase letters a-z and single spaces only; no punctuation, no numbers, no apostrophes\n"+
				"- each sentence on its own line\n"+
				"- no numbering, no bullets, no extra commentary\n"+
				"- each sentence should have at least 4 words\n",
			n, theme,
		)
	}
}
