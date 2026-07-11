package main

import (
	"strings"
	"testing"

	"github.com/christiaanswanepoel/key-mapping/core"
)

// TestRenderCorpusStatus checks that each corpus phase renders something the
// user can actually act on. A streaming corpus fills in the background, so this
// line is the only thing telling them that the text they are drilling on is
// still the fallback, that generation has kicked in, or that the model is
// unreachable — silence would be indistinguishable from a hang.
func TestRenderCorpusStatus(t *testing.T) {
	tests := []struct {
		name   string
		status core.CorpusStatus
		want   []string // substrings that must appear
	}{
		{
			name:   "ready static bank is reported plainly",
			status: core.CorpusStatus{Source: "static", Phase: core.CorpusReady, Words: 300, Sentences: 20},
			want:   []string{"static", "300", "20"},
		},
		{
			name:   "warming says it is generating and that we are on fallback text",
			status: core.CorpusStatus{Source: "ollama", Phase: core.CorpusWarming, Detail: "llama3.2"},
			want:   []string{"ollama", "generating", "llama3.2", "static"},
		},
		{
			name:   "streaming reports live counts",
			status: core.CorpusStatus{Source: "ollama", Phase: core.CorpusStreaming, Words: 240, Sentences: 60, Detail: "llama3.2"},
			want:   []string{"ollama", "streaming", "240", "60"},
		},
		{
			name:   "failure surfaces the reason and that the drill continues",
			status: core.CorpusStatus{Source: "ollama", Phase: core.CorpusFailed, Detail: "connection refused"},
			want:   []string{"ollama", "failed", "connection refused", "static"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderCorpusStatus(tt.status, 0)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("renderCorpusStatus() = %q, missing %q", got, want)
				}
			}
		})
	}
}

// TestCorpusSpinnerAdvances guards the spinner indexing: tick grows without
// bound while a corpus warms up, so the frame lookup must stay in range.
func TestCorpusSpinnerAdvances(t *testing.T) {
	status := core.CorpusStatus{Source: "ollama", Phase: core.CorpusWarming}

	seen := make(map[string]bool)
	for tick := 0; tick < 1000; tick++ {
		seen[renderCorpusStatus(status, tick)] = true // must not panic on any tick
	}
	if len(seen) < 2 {
		t.Errorf("spinner never advanced across 1000 ticks: %d distinct frames", len(seen))
	}
}

// TestCorpusSettled decides whether the TUI keeps ticking. A fixed bank must
// settle (so static/file/code sessions stay completely idle), while a failed
// stream must NOT — it retries with a backoff and can recover into streaming,
// which the user would never see if we stopped re-rendering.
func TestCorpusSettled(t *testing.T) {
	tests := []struct {
		phase core.CorpusPhase
		want  bool
	}{
		{core.CorpusReady, true},
		{core.CorpusWarming, false},
		{core.CorpusStreaming, false},
		{core.CorpusFailed, false},
	}

	for _, tt := range tests {
		st := core.State{Corpus: core.CorpusStatus{Phase: tt.phase}}
		if got := corpusSettled(st); got != tt.want {
			t.Errorf("corpusSettled(%q) = %v, want %v", tt.phase, got, tt.want)
		}
	}
}
