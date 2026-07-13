package corpus

import (
	"context"
	"fmt"
	"time"

	"github.com/ollama/ollama/types/model"
)

// DetectTimeout bounds how long Detect waits before giving up and letting the
// caller use a static bank. It only ever costs this much when a server is
// listening but wedged: nothing there at all is refused immediately.
const DetectTimeout = 2 * time.Second

// preferredModels are tried, in order, when the caller did not name one. They
// are small, fast, instruction-following models — good at "list 60 words, one
// per line", which is all the corpus asks for. Anything else pulled on the box
// is used as a fallback, so this list is a preference, not a requirement.
var preferredModels = []string{
	"llama3.2",
	"llama3.1",
	"llama3",
	"mistral",
	"qwen3",
	"phi3",
	"gemma3",
	"gemma2",
}

// Detect reports whether a usable Ollama server is reachable, and returns opts
// updated with a model that is actually pulled on it.
//
// This exists so the frontends can default to a live model when one is there
// and silently use the static bank when it is not. Two things make a naive
// "just try Ollama" check wrong, and Detect handles both:
//
//   - The server may be running but have no usable model. Options.Model
//     defaults to a name that may well not be pulled (llama3.2 on a box that
//     only has qwen3), and generating against a missing model fails. So Detect
//     resolves the model against what the server actually reports.
//   - It must be quick and it must not become a startup stall. A server that
//     is simply absent refuses the connection immediately; DetectTimeout only
//     bites when something is listening but not answering.
//
// An explicitly requested model is honoured even if the server does not list
// it — the user may be about to pull it, and the stream reports the failure
// rather than silently substituting a different model.
func Detect(ctx context.Context, opts Options) (Options, error) {
	opts = opts.withDefaults()
	explicitModel := opts.Model != defaultModel

	client, err := newOllamaClient(opts.Host)
	if err != nil {
		return opts, fmt.Errorf("corpus: detect ollama: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, DetectTimeout)
	defer cancel()

	list, err := client.List(ctx)
	if err != nil {
		return opts, fmt.Errorf("corpus: detect ollama: %w", err)
	}

	var available []string
	for _, m := range list.Models {
		if canComplete(m.Capabilities) {
			available = append(available, m.Name)
		}
	}
	if len(available) == 0 {
		return opts, fmt.Errorf("corpus: detect ollama: server has no model that can generate text")
	}

	if explicitModel {
		return opts, nil // honour the caller; a missing model surfaces as a stream failure
	}

	opts.Model = pickModel(available)
	return opts, nil
}

// canComplete reports whether a model can generate text at all. Embedding-only
// models are listed alongside the rest and would fail every request.
func canComplete(caps []model.Capability) bool {
	for _, c := range caps {
		if c == model.CapabilityCompletion {
			return true
		}
	}
	return false
}

// pickModel chooses from what is pulled: the first preferred family with a
// match, else whatever came first. Matching is on prefix so a tag ("qwen3:8b")
// matches its family ("qwen3").
func pickModel(available []string) string {
	for _, want := range preferredModels {
		for _, have := range available {
			if have == want || hasFamilyPrefix(have, want) {
				return have
			}
		}
	}
	return available[0]
}

// hasFamilyPrefix reports whether name is want at some tag, e.g. "qwen3:8b"
// for want "qwen3". A bare prefix is not enough: "qwen3-coder" is a different
// model from "qwen3", so the character after the family must be a tag
// separator.
func hasFamilyPrefix(name, want string) bool {
	return len(name) > len(want) && name[:len(want)] == want && name[len(want)] == ':'
}
