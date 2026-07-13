package corpus_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/christiaanswanepoel/key-mapping/core"
	"github.com/christiaanswanepoel/key-mapping/corpus"
)

// TestFromOllamaLive exercises FromOllama's Stream against a real, running
// Ollama server. It is opt-in: CI and a plain `go test ./...` skip it,
// because it needs a local server and a pulled model.
//
//	OLLAMA_LIVE_TEST=1 go test ./corpus -run TestFromOllamaLive -v
//	OLLAMA_LIVE_TEST=1 OLLAMA_TEST_MODEL=qwen3:8b go test ./corpus -run TestFromOllamaLive -v
//	OLLAMA_LIVE_TEST=1 OLLAMA_TEST_HOST=http://localhost:11434 go test ./corpus -run TestFromOllamaLive -v
//
// It asserts the contract the trainer relies on: the stream warms up and
// starts serving generated, mapping-safe (lowercase a-z, spaces) text within
// a reasonable timeout, without ever blocking the caller.
func TestFromOllamaLive(t *testing.T) {
	if os.Getenv("OLLAMA_LIVE_TEST") == "" {
		t.Skip("set OLLAMA_LIVE_TEST=1 (and have an Ollama server running) to run this test")
	}

	opts := corpus.Options{
		Model: os.Getenv("OLLAMA_TEST_MODEL"), // empty => package default
		Host:  os.Getenv("OLLAMA_TEST_HOST"),  // empty => OLLAMA_HOST / client default
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fallback := core.NewStaticCorpus()
	stream, err := corpus.FromOllama(ctx, opts, fallback)
	if err != nil {
		t.Fatalf("FromOllama: %v", err)
	}
	defer stream.Close()

	// Poll Status() until BOTH kinds have real generated content, rather than
	// sleeping blindly.
	//
	// Waiting on Phase alone is not enough: it flips to streaming as soon as
	// the first item of *either* kind lands, and Word/Sentence transparently
	// serve the static fallback for a kind whose buffer is still empty. A test
	// that only checked the phase would happily pass on fallback text and tell
	// us nothing about whether sentence generation works at all.
	deadline := time.Now().Add(3 * time.Minute)
	var status core.CorpusStatus
	for time.Now().Before(deadline) {
		status = stream.Status()
		if status.Phase == core.CorpusStreaming && status.Words > 0 && status.Sentences > 0 {
			break
		}
		if status.Phase == core.CorpusFailed {
			t.Fatalf("stream failed: %+v", status)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if status.Words == 0 || status.Sentences == 0 {
		t.Fatalf("stream did not generate both words and sentences within timeout, last status: %+v", status)
	}
	t.Logf("status after warmup: %+v", status)

	word := stream.Word(core.LengthAny, 0)
	if word == "" {
		t.Fatal("Word returned empty string")
	}
	if !isMappable(word) {
		t.Errorf("Word(%q) contains characters the mirror mapping cannot produce", word)
	}

	sentence := stream.Sentence(0)
	if sentence == "" {
		t.Fatal("Sentence returned empty string")
	}
	if !isMappable(sentence) {
		t.Errorf("Sentence(%q) contains characters the mirror mapping cannot produce", sentence)
	}

	// The generated text must not be the fallback bank leaking through.
	if word == fallback.Word(core.LengthAny, 0) && sentence == fallback.Sentence(0) {
		t.Errorf("got the fallback corpus's own text back (word=%q sentence=%q); "+
			"generated content is not actually being served", word, sentence)
	}

	t.Logf("word=%q sentence=%q", word, sentence)
}

// isMappable reports whether s contains only lowercase a-z and spaces — the
// characters the trainer's word/sentence drills can actually be typed with.
func isMappable(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r == ' ' {
			continue
		}
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}
