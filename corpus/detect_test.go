package corpus

import (
	"testing"

	"github.com/ollama/ollama/types/model"
)

func TestPickModel(t *testing.T) {
	tests := []struct {
		name      string
		available []string
		want      string
	}{
		{
			name:      "a preferred model wins over an unlisted one",
			available: []string{"someones-finetune:latest", "llama3.2:3b"},
			want:      "llama3.2:3b",
		},
		{
			name:      "earlier preferences win",
			available: []string{"qwen3:8b", "llama3.2:3b"},
			want:      "llama3.2:3b",
		},
		{
			name:      "a tagged model matches its family",
			available: []string{"qwen3:8b"},
			want:      "qwen3:8b",
		},
		{
			name:      "an exact untagged name matches",
			available: []string{"mistral"},
			want:      "mistral",
		},
		{
			name:      "nothing preferred: take what there is",
			available: []string{"some-exotic-model:v2"},
			want:      "some-exotic-model:v2",
		},
		{
			// The real box this was built on had only qwen3, while the package
			// default is llama3.2 — auto-detect must not pick a model that is
			// not pulled.
			name:      "picks a pulled model over the package default",
			available: []string{"qwen3:8b"},
			want:      "qwen3:8b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickModel(tt.available); got != tt.want {
				t.Errorf("pickModel(%v) = %q, want %q", tt.available, got, tt.want)
			}
		})
	}
}

// TestHasFamilyPrefix guards against matching a different model that merely
// starts with the same letters: "qwen3-coder" is not "qwen3".
func TestHasFamilyPrefix(t *testing.T) {
	tests := []struct {
		name, want string
		ok         bool
	}{
		{"qwen3:8b", "qwen3", true},
		{"llama3.2:3b", "llama3.2", true},
		{"qwen3-coder:7b", "qwen3", false}, // different model, not a tag of qwen3
		{"qwen3", "qwen3", false},          // equal, not a prefix; callers check equality separately
		{"llama3", "llama3.2", false},
	}

	for _, tt := range tests {
		if got := hasFamilyPrefix(tt.name, tt.want); got != tt.ok {
			t.Errorf("hasFamilyPrefix(%q, %q) = %v, want %v", tt.name, tt.want, got, tt.ok)
		}
	}
}

func TestCanComplete(t *testing.T) {
	if !canComplete([]model.Capability{model.CapabilityCompletion, model.CapabilityTools}) {
		t.Error("a completion model should be usable")
	}
	// Embedding models are listed alongside the rest and would fail every
	// generate request, so they must never be auto-picked.
	if canComplete([]model.Capability{model.CapabilityEmbedding}) {
		t.Error("an embedding-only model must not be considered usable")
	}
	if canComplete(nil) {
		t.Error("a model reporting no capabilities must not be considered usable")
	}
}
