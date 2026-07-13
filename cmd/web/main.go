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
	"strconv"
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
//	index.html                                     -> auto: Ollama if reachable, else static
//	index.html?corpus=static                       -> force the static bank
//	index.html?corpus=ollama                       -> force Ollama (report failure rather than downgrade)
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

	kind := q.Get("corpus")
	if kind == "" {
		kind = "auto"
	}
	if kind == "static" {
		return core.NewStaticCorpus()
	}
	if kind != "auto" && kind != "ollama" {
		return core.NewStaticCorpus()
	}

	opts := corpus.Options{
		Model:         q.Get("model"),
		Host:          q.Get("host"),
		Temperature:   floatParam(q, "temperature"),
		RepeatPenalty: floatParam(q, "repeat-penalty"),
	}

	// Explicitly requested: no probe, so nothing here blocks. FromOllama only
	// builds a client and starts a goroutine. A down server is a soft failure
	// the Stream reports through State.Corpus, which the page renders — so the
	// user sees why they are on fallback text, rather than being silently
	// downgraded.
	if kind == "ollama" {
		src, err := corpus.FromOllama(context.Background(), opts, core.NewStaticCorpus())
		if err != nil {
			return core.NewStaticCorpus() // only a malformed host reaches here
		}
		return src
	}

	// Auto: only use Ollama if it is really there with a usable model, so a page
	// opened with no Ollama running does not sit on a red "failed" status
	// forever. Detect also resolves the model to one that is actually pulled.
	//
	// Detect does I/O, and it MUST NOT run before main registers snapshot() and
	// dispatch() on the JS global: Go hands control back to the page the moment
	// main blocks, so probing here would mean the page calls snapshot() before it
	// exists ("window.snapshot is not a function") — a dead page, not a slow one.
	// So return immediately with a Deferred serving static text, and resolve it
	// on a goroutine. The page stays interactive throughout and upgrades in
	// place, which is what it already does for streamed content anyway.
	deferred := corpus.NewDeferred(core.NewStaticCorpus(), "ollama")
	go func() {
		resolved, err := corpus.Detect(context.Background(), opts)
		if err != nil {
			deferred.Abandon()
			return
		}
		src, err := corpus.FromOllama(context.Background(), resolved, core.NewStaticCorpus())
		if err != nil {
			deferred.Abandon()
			return
		}
		deferred.Resolve(src)
	}()
	return deferred
}

// floatParam reads a numeric query parameter. An absent or unparseable value
// yields 0, which corpus.Options reads as "use the default" — a typo in the URL
// must not stop the trainer from starting.
func floatParam(q url.Values, name string) float64 {
	v, err := strconv.ParseFloat(q.Get(name), 64)
	if err != nil {
		return 0
	}
	return v
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
