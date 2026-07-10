package corpus

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ollama/ollama/api"
)

// Options configures FromOllama. Zero values pick sensible defaults.
type Options struct {
	// Model is the Ollama model name to query (e.g. "llama3.2"). Defaults to
	// "llama3.2" if empty.
	Model string

	// Host is the Ollama server address (e.g. "http://localhost:11434"). If
	// empty, the client is built from the OLLAMA_HOST environment variable
	// (or its own default) via api.ClientFromEnvironment.
	Host string

	// NumWords is how many practice words to request. Defaults to 200.
	NumWords int

	// NumSentences is how many practice sentences to request. Defaults to 40.
	NumSentences int
}

const (
	defaultModel        = "llama3.2"
	defaultNumWords     = 200
	defaultNumSentences = 40

	// minWords is the minimum number of parsed words FromOllama requires
	// before it considers the response usable; below this it returns an
	// error so the caller can fall back to a static corpus.
	minWords = 10
)

func (o Options) withDefaults() Options {
	if o.Model == "" {
		o.Model = defaultModel
	}
	if o.NumWords <= 0 {
		o.NumWords = defaultNumWords
	}
	if o.NumSentences <= 0 {
		o.NumSentences = defaultNumSentences
	}
	return o
}

// FromOllama generates a word bank and sentence bank by querying a local
// Ollama server for practice text, and builds a Source from the parsed
// response. If the model's output does not yield enough usable words or any
// usable sentences, an error is returned so the caller can fall back to
// another Corpus (e.g. core.NewStaticCorpus).
func FromOllama(ctx context.Context, opts Options) (*Source, error) {
	opts = opts.withDefaults()

	client, err := newOllamaClient(opts.Host)
	if err != nil {
		return nil, fmt.Errorf("corpus: build ollama client: %w", err)
	}

	wordsRaw, err := generate(ctx, client, opts.Model, wordsPrompt(opts.NumWords))
	if err != nil {
		return nil, fmt.Errorf("corpus: generate words: %w", err)
	}

	sentencesRaw, err := generate(ctx, client, opts.Model, sentencesPrompt(opts.NumSentences))
	if err != nil {
		return nil, fmt.Errorf("corpus: generate sentences: %w", err)
	}

	words := parseWordLines(wordsRaw)
	sentences := parseSentenceLines(sentencesRaw)

	if len(words) < minWords {
		return nil, fmt.Errorf("corpus: ollama returned too few usable words (%d, want >= %d)", len(words), minWords)
	}
	if len(sentences) == 0 {
		return nil, fmt.Errorf("corpus: ollama returned no usable sentences")
	}

	return &Source{words: words, sentences: sentences}, nil
}

func newOllamaClient(host string) (*api.Client, error) {
	if host == "" {
		return api.ClientFromEnvironment()
	}
	base, err := url.Parse(host)
	if err != nil {
		return nil, err
	}
	return api.NewClient(base, http.DefaultClient), nil
}

func generate(ctx context.Context, client *api.Client, model, prompt string) (string, error) {
	stream := false
	var b strings.Builder

	req := &api.GenerateRequest{
		Model:  model,
		Prompt: prompt,
		Stream: &stream,
	}

	err := client.Generate(ctx, req, func(r api.GenerateResponse) error {
		b.WriteString(r.Response)
		return nil
	})
	if err != nil {
		return "", err
	}

	return b.String(), nil
}

func wordsPrompt(n int) string {
	return fmt.Sprintf(
		"List %d common English words for a touch-typing practice tool.\n"+
			"Rules:\n"+
			"- lowercase letters a-z only, no punctuation, no numbers, no apostrophes\n"+
			"- one word per line\n"+
			"- no numbering, no bullets, no extra commentary\n"+
			"- no duplicate words\n",
		n,
	)
}

func sentencesPrompt(n int) string {
	return fmt.Sprintf(
		"Write %d short, simple English sentences for a touch-typing practice tool.\n"+
			"Rules:\n"+
			"- lowercase letters a-z and single spaces only; no punctuation, no numbers, no apostrophes\n"+
			"- each sentence on its own line\n"+
			"- no numbering, no bullets, no extra commentary\n"+
			"- each sentence should have at least 4 words\n",
		n,
	)
}

// parseWordLines parses a raw LLM response into a clean list of practice
// words: for each line, the first [a-z]+ token is taken (after lowercasing
// and stripping numbering/punctuation), and results are deduplicated
// preserving first-seen order. Lines with no letters are skipped.
func parseWordLines(s string) []string {
	seen := make(map[string]bool)
	var words []string

	for _, line := range strings.Split(s, "\n") {
		for _, tok := range Words(line) {
			if !seen[tok] {
				seen[tok] = true
				words = append(words, tok)
			}
			break // only the first token on the line
		}
	}

	return words
}

// parseSentenceLines parses a raw LLM response into a clean list of practice
// sentences, reusing the same line-cleaning rules as Sentences.
func parseSentenceLines(s string) []string {
	return Sentences(s)
}
