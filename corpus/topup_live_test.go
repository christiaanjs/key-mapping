package corpus

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/christiaanswanepoel/key-mapping/core"
)

// TestStreamTopsUpUnderDemandLive covers the complaint that skipping a lot never
// yields new sentences.
//
// It waits for the sentence bank to SETTLE — i.e. reach its low-water mark and
// stop growing, so the producer is idle and only demand can wake it — and then
// draws one full pass through the bank, which is what a user in sentence mode
// does by skipping. Having seen every sentence once, they have earned fresh
// material, so the bank must grow.
//
// Opt-in:
//
//	OLLAMA_LIVE_TEST=1 OLLAMA_TEST_MODEL=qwen3:8b go test ./corpus -run TestStreamTopsUpUnderDemandLive -v
func TestStreamTopsUpUnderDemandLive(t *testing.T) {
	if os.Getenv("OLLAMA_LIVE_TEST") == "" {
		t.Skip("set OLLAMA_LIVE_TEST=1 (and have an Ollama server running) to run this test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	stream, err := FromOllama(ctx, Options{
		Model: os.Getenv("OLLAMA_TEST_MODEL"),
		Host:  os.Getenv("OLLAMA_TEST_HOST"),
	}, core.NewStaticCorpus())
	if err != nil {
		t.Fatalf("FromOllama: %v", err)
	}
	defer stream.Close()

	// Wait for the sentence bank to reach its low-water mark AND stop moving:
	// below the mark the producer refills regardless of demand, so any growth
	// then would prove nothing about whether skipping triggers a top-up.
	settled := waitForSettledSentences(t, stream, 6*time.Minute)
	t.Logf("sentence bank settled at %d (low-water %d)", settled, sentenceLowWater)

	// One full pass through the bank, plus a little: exactly what skipping does.
	draws := settled + 5
	for i := 0; i < draws; i++ {
		stream.Sentence(i)
	}
	t.Logf("drew %d sentences (a full pass through the bank)", draws)

	// Wait for the top-up round to run to completion, not just to start. The
	// stream ingests items as they stream in, so checking for the first sign of
	// growth would report a number from the middle of the round.
	grown := waitForSettledSentences(t, stream, 3*time.Minute)
	if grown <= settled {
		t.Fatalf("sentence bank stuck at %d after %d draws (a full pass through it): %+v\n"+
			"skipping through every sentence you have does not earn you new ones",
			settled, draws, stream.Status())
	}
	t.Logf("sentence bank grew %d -> %d (%d new)", settled, grown, grown-settled)

	// A top-up that yields only a handful of new items would mean the model is
	// mostly regenerating what we already hold; the prompt rotates themes
	// specifically to avoid that.
	if grown-settled < 5 {
		t.Errorf("top-up added only %d new sentences — generation is mostly producing duplicates",
			grown-settled)
	}
}

// waitForSettledSentences waits until the sentence bank has reached its
// low-water mark and held steady, meaning the producer has gone idle.
func waitForSettledSentences(t *testing.T, s *Stream, timeout time.Duration) int {
	t.Helper()

	deadline := time.Now().Add(timeout)
	last, stableFor := -1, time.Duration(0)

	for time.Now().Before(deadline) {
		n := s.Status().Sentences
		if n == last && n >= sentenceLowWater {
			stableFor += 500 * time.Millisecond
			if stableFor >= 5*time.Second {
				return n
			}
		} else {
			stableFor = 0
		}
		last = n
		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("sentence bank never settled at the low-water mark (%d) within %s; last = %d",
		sentenceLowWater, timeout, last)
	return 0
}
