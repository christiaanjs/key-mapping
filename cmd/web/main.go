//go:build js && wasm

// Command web is the WebAssembly entrypoint for the keymap trainer.
//
// It compiles with GOOS=js GOARCH=wasm and must not import Bubble Tea,
// Lip Gloss, or os-specific packages. It exposes two functions on the JS
// global scope, snapshot() and dispatch(eventJSON), each returning the
// current core.State serialized to JSON. All logic lives in core; this file
// only marshals across the syscall/js boundary per the architecture doc's
// "JS boundary" pattern: serialize Snapshot() to JSON once per update and
// let JS render the blob.
package main

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"syscall/js"

	"github.com/christiaanswanepoel/key-mapping/core"
	"github.com/christiaanswanepoel/key-mapping/corpus"
	"github.com/christiaanswanepoel/key-mapping/mappings"
)

func main() {
	// Parse the embedded default mapping set; fall back to the built-in
	// static mapping on error.
	m, err := core.ParseMapping(mappings.FS, mappings.Default)
	if err != nil {
		m = core.NewStaticMapping()
	}
	app := core.New(m, buildCorpus())

	js.Global().Set("snapshot", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		return stateJSON(app.Snapshot())
	}))

	js.Global().Set("dispatch", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			return stateJSON(app.Snapshot())
		}

		var ev core.Event
		if err := json.Unmarshal([]byte(args[0].String()), &ev); err != nil {
			// Malformed event from JS: ignore and just return current state.
			return stateJSON(app.Snapshot())
		}

		return stateJSON(app.Dispatch(ev))
	}))

	select {} // keep the runtime alive
}

// buildCorpus selects the corpus from the page's query string — the browser's
// equivalent of the TUI's flags, injected the same way at construction:
//
//	index.html                                     -> static bank (default)
//	index.html?corpus=ollama                       -> stream from a local Ollama
//	index.html?corpus=ollama&model=qwen3:8b        -> ...with a specific model
//	index.html?corpus=ollama&host=http://host:1234 -> ...on a specific server
//
// A browser genuinely can reach a local Ollama server, contrary to what this
// file used to assume: Ollama's default CORS policy allows localhost origins,
// and Go's wasm net/http transport goes through fetch(), which streams the
// response body — so corpus.Stream works here exactly as it does natively,
// producer goroutine and all. Both of those require the page to be served over
// HTTP from localhost (under file:// the origin is "null", which Ollama
// rejects — and instantiateStreaming needs HTTP regardless).
//
// There is deliberately no file/codebase option: those need a filesystem the
// sandbox does not have. Anything unrecognized serves the static bank — the
// trainer must always start.
func buildCorpus() core.Corpus {
	q := queryParams()

	if q.Get("corpus") != "ollama" {
		return core.NewStaticCorpus()
	}

	// The stream outlives this call, generating on a background goroutine for
	// as long as the page is open; context.Background is right because the
	// page's lifetime is the program's lifetime.
	src, err := corpus.FromOllama(context.Background(), corpus.Options{
		Model: q.Get("model"),
		Host:  q.Get("host"),
	}, core.NewStaticCorpus())
	if err != nil {
		// Only a malformed host reaches here. An unreachable server is a soft
		// failure the Stream reports through State.Corpus, which the page
		// renders — so the user sees why they are on fallback text.
		return core.NewStaticCorpus()
	}
	return src
}

// queryParams reads window.location.search. Anything unexpected — no location
// object at all (this binary can be driven outside a browser, e.g. under Node
// for testing), or a malformed query string — yields an empty set rather than
// a panic across the JS boundary. The only consequence is the static bank.
func queryParams() url.Values {
	loc := js.Global().Get("location")
	if !loc.Truthy() {
		return url.Values{}
	}
	search := loc.Get("search")
	if search.Type() != js.TypeString {
		return url.Values{}
	}
	v, err := url.ParseQuery(strings.TrimPrefix(search.String(), "?"))
	if err != nil {
		return url.Values{}
	}
	return v
}

// stateJSON marshals a core.State to its JSON string form. Marshal errors on
// this type are not expected (it is plain data with json tags), so on the
// rare failure we fall back to an empty object rather than panicking across
// the JS boundary.
func stateJSON(st core.State) string {
	b, err := json.Marshal(st)
	if err != nil {
		return "{}"
	}
	return string(b)
}
