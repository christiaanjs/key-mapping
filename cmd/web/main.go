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
	"encoding/json"
	"syscall/js"

	"github.com/christiaanswanepoel/key-mapping/core"
	"github.com/christiaanswanepoel/key-mapping/mappings"
)

func main() {
	// Parse the embedded default mapping set; fall back to the built-in
	// static mapping on error. The corpus always stays static in wasm: a
	// browser sandbox cannot reach a local Ollama server.
	m, err := core.ParseMapping(mappings.FS, mappings.Default)
	if err != nil {
		m = core.NewStaticMapping()
	}
	app := core.New(m, core.NewStaticCorpus())

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
